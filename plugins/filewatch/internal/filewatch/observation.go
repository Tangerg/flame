package filewatch

import (
	json "encoding/json/v2"
	"errors"
	"slices"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/exactint"
	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/identity"
	framecall "github.com/Tangerg/frame/call"
	"github.com/google/uuid"
)

type Kind string

const (
	Changed Kind = "changed"
	Resync  Kind = "resync"
)

type Capture struct {
	Generation string               `json:"generation"`
	Sequence   uint64               `json:"sequence"`
	Revision   int64                `json:"revision"`
	Kind       Kind                 `json:"kind"`
	Paths      []string             `json:"paths"`
	ObservedAt time.Time            `json:"observedAt"`
	Trigger    Draft                `json:"trigger"`
	SessionID  identity.SessionID   `json:"sessionId"`
	RunID      identity.RunID       `json:"runId"`
	Invocation framecall.Invocation `json:"invocation"`
}

type Observation struct{ capture Capture }

func NewObservation(capture Capture) (Observation, error) {
	if _, err := uuid.Parse(capture.Generation); err != nil {
		return Observation{}, ErrInvalid
	}
	if !capture.positioned() {
		return Observation{}, ErrInvalid
	}
	trigger, err := newTrigger(capture.Trigger)
	if err != nil {
		return Observation{}, err
	}
	if capture.SessionID.IsZero() || capture.RunID.IsZero() {
		return Observation{}, ErrInvalid
	}
	if err := extension.ValidateInvocation(capture.Invocation); err != nil {
		return Observation{}, errors.Join(ErrInvalid, err)
	}
	if capture.Invocation.Origin != extension.Automatic || capture.Invocation.Parent != "" || capture.Invocation.Target.Scope != extension.ScopeInstallation {
		return Observation{}, ErrInvalid
	}
	if !capture.observesTriggerPaths() {
		return Observation{}, ErrInvalid
	}
	capture.Trigger = trigger.Draft()
	capture.Paths = slices.Clone(capture.Paths)
	capture.ObservedAt = capture.ObservedAt.UTC()
	return Observation{capture: capture}, nil
}

func (c Capture) positioned() bool {
	return c.Sequence != 0 && c.Sequence <= uint64(exactint.Maximum) && c.Revision >= 1 && c.Revision <= exactint.Maximum && !c.ObservedAt.IsZero() &&
		slices.Contains(c.Kind.JSONSchemaEnum(), string(c.Kind)) && len(c.Paths) != 0 && len(c.Paths) <= maxPaths
}

func (c Capture) observesTriggerPaths() bool {
	seen := make(map[string]bool, len(c.Paths))
	for _, path := range c.Paths {
		if !slices.Contains(c.Trigger.Paths, path) || seen[path] {
			return false
		}
		seen[path] = true
	}
	return c.Kind != Resync || slices.Equal(c.Paths, c.Trigger.Paths)
}

func (o Observation) Capture() Capture {
	value := o.capture
	value.Paths = slices.Clone(value.Paths)
	value.Trigger.Paths = slices.Clone(value.Trigger.Paths)
	return value
}

func (o Observation) Input() (string, error) {
	context, err := json.Marshal(struct {
		Kind       Kind      `json:"kind"`
		Paths      []string  `json:"paths"`
		ObservedAt time.Time `json:"observedAt"`
	}{o.capture.Kind, o.capture.Paths, o.capture.ObservedAt}, json.Deterministic(true))
	if err != nil {
		return "", err
	}
	return o.capture.Trigger.Instructions + "\n\n<flame-file-observation>\n" + string(context) + "\n</flame-file-observation>", nil
}

type Event struct {
	Capture  Capture `json:"capture"`
	Accepted bool    `json:"accepted"`
}

type Header struct {
	ID         string             `json:"id"`
	Revision   int64              `json:"revision"`
	Kind       Kind               `json:"kind"`
	ObservedAt time.Time          `json:"observedAt"`
	PathCount  int                `json:"pathCount"`
	SessionID  identity.SessionID `json:"sessionId"`
	RunID      identity.RunID     `json:"runId"`
	Accepted   bool               `json:"accepted"`
}

func (e Event) Header() Header {
	return Header{ID: e.Capture.Invocation.ID, Revision: e.Capture.Revision, Kind: e.Capture.Kind, ObservedAt: e.Capture.ObservedAt,
		PathCount: len(e.Capture.Paths), SessionID: e.Capture.SessionID, RunID: e.Capture.RunID, Accepted: e.Accepted}
}

type Phase string

const (
	Inactive Phase = "inactive"
	Disabled Phase = "disabled"
	Watching Phase = "watching"
	Failed   Phase = "failed"
)

type Status struct {
	Revision      int64  `json:"revision"`
	Phase         Phase  `json:"phase"`
	Error         string `json:"error,omitempty"`
	DispatchError string `json:"dispatchError,omitempty"`
}

type Cursor struct {
	generation string
	sequence   uint64
}

func NewCursor(generation string) (Cursor, error) {
	if _, err := uuid.Parse(generation); err != nil {
		return Cursor{}, ErrInvalid
	}
	return Cursor{generation: generation}, nil
}

func (c *Cursor) Advance(sequence uint64) (bool, error) {
	if c.generation == "" || sequence <= c.sequence || sequence > uint64(exactint.Maximum) {
		return false, errors.New("filewatch: invalid observation sequence")
	}
	gap := sequence != c.sequence+1
	c.sequence = sequence
	return gap, nil
}

func (Kind) JSONSchemaEnum() []string {
	return []string{string(Changed), string(Resync)}
}

func (Phase) JSONSchemaEnum() []string {
	return []string{string(Inactive), string(Disabled), string(Watching), string(Failed)}
}
