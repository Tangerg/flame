package goal

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/exactint"
	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/goalworkflow"
	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/packages/go-sdk/instant"
	flamemodel "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/runtime/internal/domain/automation/goalref"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
)

type Status string

const (
	StatusActive  Status = Status(goalworkflow.GoalActive)
	StatusPaused  Status = Status(goalworkflow.GoalPaused)
	StatusBlocked Status = Status(goalworkflow.GoalBlocked)
	// The workflow reports a finishing Goal as completing; the Runtime records
	// the settled terminal state.
	StatusComplete Status = "complete"
)

func (s Status) Valid() bool {
	switch s {
	case StatusActive, StatusPaused, StatusBlocked, StatusComplete:
		return true
	default:
		return false
	}
}

type Snapshot struct {
	Author            extension.ContributionSource
	SessionID         identity.SessionID
	Objective         string
	Status            Status
	ReasonCode        ReasonCode
	ReasonRunOutcome  run.Outcome
	ReasonExplanation string
	ModelSelection    flamemodel.Selection
	Capabilities      run.Capabilities
	Used              Usage
	IncarnationID     goalref.IncarnationID
	Revision          uint64
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type Goal struct {
	author        extension.ContributionSource
	sessionID     identity.SessionID
	objective     string
	status        Status
	reason        Reason
	selection     flamemodel.Selection
	capabilities  run.Capabilities
	used          Usage
	incarnationID goalref.IncarnationID
	revision      exactint.Counter
	createdAt     time.Time
	updatedAt     time.Time
}

type Current struct {
	sessionID identity.SessionID
	goal      *Goal
}

type Version struct {
	author        extension.ContributionSource
	sessionID     identity.SessionID
	incarnationID goalref.IncarnationID
	revision      exactint.Counter
	committed     bool
}

var (
	errSessionRequired          = errors.New("goal: session ID is required")
	errObjectiveRequired        = errors.New("goal: objective is required")
	ErrInvalid                  = errors.New("goal: invalid Goal")
	ErrNotResumable             = errors.New("goal: status is not resumable")
	ErrNotEditable              = errors.New("goal: status is not editable")
	errRunIdentityConflict      = errors.New("goal: Run identity conflict")
	ErrBlockExplanationRequired = fmt.Errorf("%w: model block requires an explanation", ErrInvalid)
)

func Unwritten(sessionID identity.SessionID) (Current, error) {
	if err := validateSessionIdentity(sessionID); err != nil {
		return Current{}, err
	}
	return Current{sessionID: sessionID}, nil
}

func CurrentOf(value Goal) (Current, error) {
	if err := value.validate(); err != nil {
		return Current{}, err
	}
	owned := value.Clone()
	return Current{sessionID: owned.sessionID, goal: &owned}, nil
}

func (c Current) Goal() (Goal, bool) {
	if c.goal == nil {
		return Goal{}, false
	}
	return c.goal.Clone(), true
}

func (c Current) Version() Version {
	if c.goal == nil {
		return Version{sessionID: c.sessionID}
	}
	return c.goal.Version()
}

func Restore(snapshot Snapshot) (Goal, error) {
	revision, err := exactint.Restore(snapshot.Revision)
	if err != nil {
		return Goal{}, fmt.Errorf("%w: revision: %w", ErrInvalid, err)
	}
	reason := Reason{}
	if snapshot.ReasonCode != ReasonNone || snapshot.ReasonRunOutcome != "" || snapshot.ReasonExplanation != "" {
		var err error
		reason, err = newReason(snapshot.Status, snapshot.ReasonCode, snapshot.ReasonRunOutcome, snapshot.ReasonExplanation)
		if err != nil {
			return Goal{}, err
		}
	}
	value := Goal{author: snapshot.Author,
		sessionID:     snapshot.SessionID,
		objective:     snapshot.Objective,
		status:        snapshot.Status,
		reason:        reason,
		selection:     snapshot.ModelSelection,
		capabilities:  snapshot.Capabilities.Clone(),
		used:          snapshot.Used,
		incarnationID: snapshot.IncarnationID,
		revision:      revision,
		createdAt:     snapshot.CreatedAt.UTC(),
		updatedAt:     snapshot.UpdatedAt.UTC(),
	}
	if err := value.validate(); err != nil {
		return Goal{}, err
	}
	return value, nil
}

func validateObjective(objective string) error {
	if strings.TrimSpace(objective) == "" {
		return errObjectiveRequired
	}
	if objective != strings.TrimSpace(objective) {
		return fmt.Errorf("%w: objective has surrounding whitespace", ErrInvalid)
	}
	return nil
}

func (g Goal) validate() error {
	if g.author != (extension.ContributionSource{}) {
		if err := g.author.Validate(); err != nil {
			return fmt.Errorf("%w: author: %w", ErrInvalid, err)
		}
	}

	if err := validateSessionIdentity(g.sessionID); err != nil {
		return err
	}
	if err := validateObjective(g.objective); err != nil {
		return err
	}
	if err := g.incarnationID.Validate(); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	if g.revision.IsZero() {
		return fmt.Errorf("%w: revision must be positive", ErrInvalid)
	}
	if !g.status.Valid() {
		return fmt.Errorf("%w: unknown status %q", ErrInvalid, g.status)
	}
	if g.selection.Configured() {
		if err := g.selection.ValidateExact(); err != nil {
			return fmt.Errorf("%w: %w", ErrInvalid, err)
		}
	}
	if err := g.capabilities.Validate(); err != nil {
		return fmt.Errorf("%w: capabilities: %w", ErrInvalid, err)
	}
	if err := g.used.validate(); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	if err := g.validateTimes(); err != nil {
		return err
	}
	return g.reason.validateFor(g.status)
}

func (g Goal) validateTimes() error {
	if g.createdAt.IsZero() || g.updatedAt.IsZero() {
		return fmt.Errorf("%w: creation and update times are required", ErrInvalid)
	}
	if err := instant.Validate(g.createdAt, g.updatedAt); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	if g.updatedAt.Before(g.createdAt) {
		return fmt.Errorf("%w: update time precedes creation", ErrInvalid)
	}
	return nil
}

func (g Goal) Snapshot() Snapshot {
	return Snapshot{Author: g.author,
		SessionID: g.sessionID, Objective: g.objective, Status: g.status,
		ReasonCode: g.reason.code, ReasonRunOutcome: g.reason.runOutcome, ReasonExplanation: g.reason.explanation,
		ModelSelection: g.selection, Capabilities: g.capabilities.Clone(),
		Used: g.used, IncarnationID: g.incarnationID,
		Revision: g.revision.Value(), CreatedAt: g.createdAt, UpdatedAt: g.updatedAt,
	}
}

func (g Goal) Clone() Goal {
	g.capabilities = g.capabilities.Clone()
	return g
}

func (g Goal) SessionID() identity.SessionID { return g.sessionID }
func (g Goal) Objective() string             { return g.objective }
func (g Goal) Status() Status                { return g.status }
func (g Goal) Reason() Reason                { return g.reason }

func (g Goal) ModelSelection() flamemodel.Selection { return g.selection }
func (g Goal) Capabilities() run.Capabilities       { return g.capabilities.Clone() }
func (g Goal) Used() Usage                          { return g.used }
func (g Goal) IncarnationID() goalref.IncarnationID { return g.incarnationID }
func (g Goal) Revision() uint64                     { return g.revision.Value() }
func (g Goal) CreatedAt() time.Time                 { return g.createdAt }
func (g Goal) UpdatedAt() time.Time                 { return g.updatedAt }

func (g Goal) Version() Version {
	return Version{author: g.author, sessionID: g.sessionID, incarnationID: g.incarnationID, revision: g.revision, committed: true}
}

func (v Version) IsUnwritten() bool                            { return !v.committed }
func (v Version) SessionID() identity.SessionID                { return v.sessionID }
func (v Version) IncarnationID() (goalref.IncarnationID, bool) { return v.incarnationID, v.committed }
func (v Version) Revision() (uint64, bool)                     { return v.revision.Value(), v.committed }

func (v Version) Validate() error {
	if err := validateSessionIdentity(v.sessionID); err != nil {
		return err
	}
	if !v.committed {
		if !v.incarnationID.IsZero() || !v.revision.IsZero() {
			return fmt.Errorf("%w: unwritten Version carries committed identity", ErrInvalid)
		}
		return nil
	}
	if err := v.incarnationID.Validate(); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	if v.revision.IsZero() {
		return fmt.Errorf("%w: committed Version revision must be positive", ErrInvalid)
	}
	return nil
}

func (v Version) AdvancesTo(next Goal) error {
	if err := v.Validate(); err != nil {
		return err
	}
	if err := next.validate(); err != nil {
		return err
	}
	if next.sessionID != v.sessionID {
		return fmt.Errorf("%w: replacement belongs to another Session", ErrInvalid)
	}
	if v.committed && v.incarnationID == next.incarnationID && v.author != next.author {
		return fmt.Errorf("%w: incarnation author changed", ErrInvalid)
	}
	if !v.committed || next.incarnationID != v.incarnationID {
		if next.revision != exactint.First() {
			return fmt.Errorf("%w: fresh incarnation must start at revision %d", ErrInvalid, exactint.First().Value())
		}
		return nil
	}
	if err := exactint.Follows(v.revision.Value(), next.revision.Value()); err != nil {
		return fmt.Errorf("%w: replacement revision %d does not follow %s: %w", ErrInvalid, next.revision.Value(), v, err)
	}
	return nil
}

func (v Version) String() string {
	if !v.committed {
		return fmt.Sprintf("%s/unwritten", v.sessionID)
	}
	return fmt.Sprintf("%s/%s@%d", v.sessionID, v.incarnationID.String(), v.revision.Value())
}

func validateSessionIdentity(value identity.SessionID) error {
	if value.IsZero() {
		return errSessionRequired
	}
	return nil
}
