package schedule

import (
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	"github.com/Tangerg/flame/runtime/internal/exactint"
)

// OccurrenceSnapshot is the complete durable representation of one cron
// firing after its schedule cursor claim has succeeded. Its ID names the
// Schedule and the due cursor it consumed, so neither is stored beside it.
type OccurrenceSnapshot struct {
	ID        string
	Execution ExecutionSnapshot
	FiredAt   time.Time
	NextRunAt time.Time
	SessionID string
	RunID     string
}

// Occurrence is one durable cron firing intent. It captures only the execution
// value plus the Schedule CAS/cursor facts needed to claim exactly once.
type Occurrence struct {
	id        occurrenceIdentity
	execution Execution
	firedAt   time.Time
	nextRunAt time.Time
	sessionID string
	runID     string
}

// Claim binds one immutable occurrence to the exact Schedule revision and due
// cursor it intends to consume. A claimed/pending Occurrence no longer carries
// this precondition, so zero never doubles as a lifecycle marker.
type Claim struct {
	occurrence       Occurrence
	expectedRevision exactint.Counter
}

// Acceptance identifies the exact durable occurrence and Run whose opening
// committed together. Private fields prevent argument transposition and
// partial ownership proofs at the persistence boundary.
type Acceptance struct {
	occurrenceID occurrenceIdentity
	runID        string
}

// NewClaim captures one due cursor and derives the next cursor from the firing
// instant. Stable Run identities are supplied before persistence so a
// crash/retry cannot create a second Run.
func NewClaim(s Schedule, sessionID, runID string, firedAt time.Time) (Claim, error) {
	if !s.Enabled() {
		return Claim{}, errors.New("schedule: only an enabled due schedule can form an occurrence")
	}
	firedAt = canonicalTime(firedAt)
	if firedAt.IsZero() {
		return Claim{}, errors.New("schedule: occurrence firing time is required")
	}
	nextRunAt, err := NextRun(s.cron, firedAt)
	if err != nil {
		return Claim{}, err
	}
	occurrenceID, err := newOccurrenceIdentity(s.id, s.nextRunAt)
	if err != nil {
		return Claim{}, err
	}
	occurrence := Occurrence{
		id: occurrenceID, execution: s.Execution(), firedAt: firedAt,
		nextRunAt: nextRunAt, sessionID: sessionID, runID: runID,
	}
	value := Claim{occurrence: occurrence, expectedRevision: s.revision}
	if err := value.Validate(); err != nil {
		return Claim{}, err
	}
	return value, nil
}

// Validate verifies the immutable durable firing value.
func (o Occurrence) Validate() error {
	if err := o.id.Validate(); err != nil {
		return err
	}
	if o.sessionID == "" || o.runID == "" {
		return errors.New("schedule: occurrence identities are required")
	}
	if err := resourceid.ValidateSession(o.sessionID); err != nil {
		return fmt.Errorf("schedule: occurrence: %w", err)
	}
	if err := resourceid.ValidateRun(o.runID); err != nil {
		return fmt.Errorf("schedule: occurrence: %w", err)
	}
	if err := o.execution.Validate(); err != nil {
		return err
	}
	if o.firedAt.IsZero() || o.nextRunAt.IsZero() {
		return errors.New("schedule: occurrence times are required")
	}
	dueAt := o.DueAt()
	if o.firedAt.Before(dueAt) {
		return errors.New("schedule: occurrence fired before it was due")
	}
	if !o.nextRunAt.After(dueAt) || !o.nextRunAt.After(o.firedAt) {
		return errors.New("schedule: occurrence next cursor must follow its history")
	}
	return nil
}

// Validate verifies the claim's occurrence and exact non-zero CAS identity.
func (c Claim) Validate() error {
	if err := c.occurrence.Validate(); err != nil {
		return err
	}
	if c.expectedRevision.IsZero() {
		return ErrRevisionRequired
	}
	return nil
}

// RestoreOccurrence reconstructs a durable pending firing without exposing a
// mutable half-aggregate to callers that reconstruct stored state.
func RestoreOccurrence(snapshot OccurrenceSnapshot) (Occurrence, error) {
	occurrenceID, err := parseOccurrenceIdentity(snapshot.ID)
	if err != nil {
		return Occurrence{}, err
	}
	execution, err := RestoreExecution(snapshot.Execution)
	if err != nil {
		return Occurrence{}, err
	}
	value := Occurrence{
		id: occurrenceID, execution: execution, firedAt: canonicalTime(snapshot.FiredAt),
		nextRunAt: canonicalTime(snapshot.NextRunAt), sessionID: snapshot.SessionID,
		runID: snapshot.RunID,
	}
	if err := value.Validate(); err != nil {
		return Occurrence{}, err
	}
	return value, nil
}

// Snapshot returns the complete durable occurrence representation.
func (o Occurrence) Snapshot() OccurrenceSnapshot {
	return OccurrenceSnapshot{
		ID: o.id.String(), Execution: o.execution.Snapshot(),
		FiredAt: o.firedAt, NextRunAt: o.nextRunAt,
		SessionID: o.sessionID, RunID: o.runID,
	}
}

func (o Occurrence) ID() string           { return o.id.String() }
func (o Occurrence) ScheduleID() string   { return o.id.scheduleID.String() }
func (o Occurrence) Execution() Execution { return o.execution }
func (o Occurrence) DueAt() time.Time     { return time.UnixMilli(o.id.dueMillis).UTC() }
func (o Occurrence) FiredAt() time.Time   { return o.firedAt }
func (o Occurrence) NextRunAt() time.Time { return o.nextRunAt }
func (o Occurrence) SessionID() string    { return o.sessionID }
func (o Occurrence) RunID() string        { return o.runID }

// Occurrence returns the immutable firing produced if this claim commits.
func (c Claim) Occurrence() Occurrence { return c.occurrence }

// ExpectedRevision returns the exact Schedule revision this claim consumes.
func (c Claim) ExpectedRevision() uint64 { return c.expectedRevision.Value() }

// Validate rejects partial occurrence ownership.
func (a Acceptance) Validate() error {
	if err := a.occurrenceID.Validate(); err != nil {
		return err
	}
	if a.runID == "" {
		return errors.New("schedule: acceptance identities are required")
	}
	if err := resourceid.ValidateRun(a.runID); err != nil {
		return fmt.Errorf("schedule: acceptance: %w", err)
	}
	return nil
}

func (a Acceptance) OccurrenceID() string { return a.occurrenceID.String() }
func (a Acceptance) RunID() string        { return a.runID }
