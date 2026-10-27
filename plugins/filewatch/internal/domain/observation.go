package filewatch

import (
	json "encoding/json/v2"
	"errors"
	"github.com/Tangerg/flame/qimu/portable"
	"slices"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/exactint"
	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/google/uuid"
)

type Kind string

const (
	Changed Kind = "changed"
	Resync  Kind = "resync"
)

// A connection-local sequence cannot identify an observation after reconnection.
// The generation and immutable capture keep retries bound to the original work.
type Capture struct {
	Generation string              `json:"generation"`
	Sequence   uint64              `json:"sequence"`
	Revision   int64               `json:"revision"`
	Kind       Kind                `json:"kind"`
	Paths      []string            `json:"paths"`
	ObservedAt time.Time           `json:"observedAt"`
	Trigger    Draft               `json:"trigger"`
	SessionID  string              `json:"sessionId"`
	RunID      string              `json:"runId"`
	Invocation portable.Invocation `json:"invocation"`
}

type Observation struct{ capture Capture }

func NewObservation(capture Capture) (Observation, error) {
	if _, err := uuid.Parse(capture.Generation); err != nil {
		return Observation{}, ErrInvalid
	}
	if capture.Sequence == 0 || capture.Sequence > uint64(exactint.Maximum) || capture.Revision < 1 || capture.Revision > exactint.Maximum || capture.ObservedAt.IsZero() ||
		!slices.Contains(capture.Kind.JSONSchemaEnum(), string(capture.Kind)) || len(capture.Paths) == 0 || len(capture.Paths) > maxPaths {
		return Observation{}, ErrInvalid
	}
	trigger, err := newTrigger(capture.Trigger)
	if err != nil {
		return Observation{}, err
	}
	if err := errors.Join(identity.ValidateSession(capture.SessionID), identity.ValidateRun(capture.RunID), extension.ValidateInvocation(capture.Invocation)); err != nil {
		return Observation{}, errors.Join(ErrInvalid, err)
	}
	if capture.Invocation.Origin != extension.Automatic || capture.Invocation.Parent != "" || capture.Invocation.Target.Scope != "installation" {
		return Observation{}, ErrInvalid
	}
	seen := make(map[string]bool, len(capture.Paths))
	for _, path := range capture.Paths {
		if !slices.Contains(capture.Trigger.Paths, path) || seen[path] {
			return Observation{}, ErrInvalid
		}
		seen[path] = true
	}
	if capture.Kind == Resync && !slices.Equal(capture.Paths, capture.Trigger.Paths) {
		return Observation{}, ErrInvalid
	}
	capture.Trigger = trigger.Draft()
	capture.Paths = slices.Clone(capture.Paths)
	capture.ObservedAt = capture.ObservedAt.UTC()
	return Observation{capture: capture}, nil
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
	ID         string    `json:"id"`
	Revision   int64     `json:"revision"`
	Kind       Kind      `json:"kind"`
	ObservedAt time.Time `json:"observedAt"`
	PathCount  int       `json:"pathCount"`
	SessionID  string    `json:"sessionId"`
	RunID      string    `json:"runId"`
	Accepted   bool      `json:"accepted"`
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
