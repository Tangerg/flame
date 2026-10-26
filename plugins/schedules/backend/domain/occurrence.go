package schedule

import (
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/capability"

	"github.com/Tangerg/flame/packages/go-sdk/exactint"
	"github.com/Tangerg/flame/packages/go-sdk/resourceid"
)

// The ID already encodes Schedule and due cursor; storing either again would add a competing representation.
type OccurrenceSnapshot struct {
	ID         string
	Execution  ExecutionSnapshot
	FiredAt    time.Time
	NextRunAt  time.Time
	SessionID  string
	RunID      string
	Invocation capability.Invocation
}

type Occurrence struct {
	id         occurrenceIdentity
	execution  Execution
	firedAt    time.Time
	nextRunAt  time.Time
	sessionID  string
	runID      string
	invocation capability.Invocation
}

// The consumed revision is a claim precondition; pending work retains only the captured execution.
type Claim struct {
	occurrence       Occurrence
	expectedRevision exactint.Counter
	state            Schedule
}

// Backend acknowledgement cannot depend on the Runtime Run still existing.
type Acceptance struct {
	occurrenceID occurrenceIdentity
	runID        string
}

// Stable Session/Run identities are chosen before persistence so retry cannot open a second Run.
func (s Schedule) Claim(sessionID, runID string, firedAt time.Time, invocation capability.Invocation) (Claim, error) {
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
		invocation: invocation,
	}
	if err := occurrence.Validate(); err != nil {
		return Claim{}, err
	}
	state, err := s.advanceRevision()
	if err != nil {
		return Claim{}, err
	}
	state.nextRunAt = nextRunAt
	return Claim{occurrence: occurrence, expectedRevision: s.revision, state: state}, nil
}

func (o Occurrence) Validate() error {
	if err := o.invocation.Validate(); err != nil {
		return err
	}
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
	execution, err := RestoreExecution(snapshot.Execution)
	if err != nil {
		return Occurrence{}, err
	}
	value := Occurrence{
		id: occurrenceID, execution: execution, firedAt: canonicalTime(snapshot.FiredAt),
		nextRunAt: canonicalTime(snapshot.NextRunAt), sessionID: snapshot.SessionID,
		runID:      snapshot.RunID,
		invocation: snapshot.Invocation,
	}
	if err := value.Validate(); err != nil {
		return Occurrence{}, err
	}
	return value, nil
}

func (o Occurrence) Snapshot() OccurrenceSnapshot {
	return OccurrenceSnapshot{
		ID: o.id.String(), Execution: o.execution.Snapshot(),
		FiredAt: o.firedAt, NextRunAt: o.nextRunAt,
		SessionID: o.sessionID, RunID: o.runID,
		Invocation: o.invocation,
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

func (c Claim) Occurrence() Occurrence { return c.occurrence }

func (c Claim) ExpectedRevision() uint64 { return c.expectedRevision.Value() }

func (c Claim) State() Schedule { return c.state }

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

func ParseAcceptance(occurrenceID, runID string) (Acceptance, error) {
	identity, err := parseOccurrenceIdentity(occurrenceID)
	if err != nil {
		return Acceptance{}, err
	}
	value := Acceptance{occurrenceID: identity, runID: runID}
	return value, value.Validate()
}

const MaximumDispatchBatch = 32
