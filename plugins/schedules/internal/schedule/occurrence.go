package schedule

import (
	"errors"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/exactint"
	"github.com/Tangerg/flame/packages/go-sdk/extension"
	flameidentity "github.com/Tangerg/flame/packages/go-sdk/identity"
	framecall "github.com/Tangerg/frame/call"
)

type OccurrenceSnapshot struct {
	ID         string
	Execution  ExecutionSnapshot
	FiredAt    time.Time
	NextRunAt  time.Time
	SessionID  flameidentity.SessionID
	RunID      flameidentity.RunID
	Invocation framecall.Invocation
}

type Occurrence struct {
	id         occurrenceIdentity
	execution  Execution
	firedAt    time.Time
	nextRunAt  time.Time
	sessionID  flameidentity.SessionID
	runID      flameidentity.RunID
	invocation framecall.Invocation
}

type Claim struct {
	occurrence       Occurrence
	expectedRevision exactint.Counter
	state            Schedule
}

type Acceptance struct {
	occurrenceID occurrenceIdentity
	runID        flameidentity.RunID
}

// Stable Session/Run identities are chosen before persistence so retry cannot open a second Run.
func (s Schedule) Claim(sessionID flameidentity.SessionID, runID flameidentity.RunID, firedAt time.Time, invocation framecall.Invocation) (Claim, error) {
	if !s.Enabled() {
		return Claim{}, errors.New("schedule: only an enabled due schedule can form an occurrence")
	}
	firedAt = canonicalTime(firedAt)
	if firedAt.IsZero() {
		return Claim{}, errors.New("schedule: occurrence firing time is required")
	}
	nextRunAt, err := s.recurrence.next(firedAt)
	if err != nil {
		return Claim{}, err
	}
	occurrenceID, err := newOccurrenceIdentity(s.id, s.nextRunAt)
	if err != nil {
		return Claim{}, err
	}
	occurrence := Occurrence{
		id: occurrenceID, execution: s.execution(), firedAt: firedAt,
		nextRunAt: nextRunAt, sessionID: sessionID, runID: runID,
		invocation: invocation,
	}
	if err := occurrence.validate(); err != nil {
		return Claim{}, err
	}
	state, err := s.advanceRevision()
	if err != nil {
		return Claim{}, err
	}
	state.nextRunAt = nextRunAt
	return Claim{occurrence: occurrence, expectedRevision: s.revision, state: state}, nil
}

func (o Occurrence) validate() error {
	if err := extension.ValidateInvocation(o.invocation); err != nil {
		return err
	}
	if err := o.id.validate(); err != nil {
		return err
	}
	if o.sessionID.IsZero() || o.runID.IsZero() {
		return errors.New("schedule: occurrence identities are required")
	}
	if err := o.execution.validate(); err != nil {
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

func (c Claim) Validate() error {
	if c.expectedRevision.IsZero() {
		return ErrRevisionRequired
	}
	return nil
}

func RestoreOccurrence(snapshot OccurrenceSnapshot) (Occurrence, error) {
	occurrenceID, err := parseOccurrenceIdentity(snapshot.ID)
	if err != nil {
		return Occurrence{}, err
	}
	execution, err := restoreExecution(snapshot.Execution)
	if err != nil {
		return Occurrence{}, err
	}
	value := Occurrence{
		id: occurrenceID, execution: execution, firedAt: canonicalTime(snapshot.FiredAt),
		nextRunAt: canonicalTime(snapshot.NextRunAt), sessionID: snapshot.SessionID,
		runID:      snapshot.RunID,
		invocation: snapshot.Invocation,
	}
	if err := value.validate(); err != nil {
		return Occurrence{}, err
	}
	return value, nil
}

func (o Occurrence) Snapshot() OccurrenceSnapshot {
	return OccurrenceSnapshot{
		ID: o.id.String(), Execution: o.execution.snapshot(),
		FiredAt: o.firedAt, NextRunAt: o.nextRunAt,
		SessionID: o.sessionID, RunID: o.runID,
		Invocation: o.invocation,
	}
}

func (o Occurrence) ID() string         { return o.id.String() }
func (o Occurrence) ScheduleID() string { return o.id.scheduleID.String() }
func (o Occurrence) DueAt() time.Time   { return time.UnixMilli(o.id.dueMillis).UTC() }

func (c Claim) Occurrence() Occurrence { return c.occurrence }

func (c Claim) ExpectedRevision() uint64 { return c.expectedRevision.Value() }

func (c Claim) State() Schedule { return c.state }

func (a Acceptance) Validate() error {
	if err := a.occurrenceID.validate(); err != nil {
		return err
	}
	if a.runID.IsZero() {
		return errors.New("schedule: acceptance identities are required")
	}
	return nil
}

func (a Acceptance) OccurrenceID() string       { return a.occurrenceID.String() }
func (a Acceptance) RunID() flameidentity.RunID { return a.runID }

func ParseAcceptance(occurrenceID string, runID flameidentity.RunID) (Acceptance, error) {
	identity, err := parseOccurrenceIdentity(occurrenceID)
	if err != nil {
		return Acceptance{}, err
	}
	value := Acceptance{occurrenceID: identity, runID: runID}
	return value, value.Validate()
}

const MaximumDispatchBatch = 32
