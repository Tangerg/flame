package schedule

import (
	"errors"
	"fmt"
	"github.com/Tangerg/flame/runtime/capability"
	"time"

	"github.com/Tangerg/flame/runtime/internal/optional"

	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
)

type RunRecord struct {
	scheduleID scheduleIdentity
	ranAt      time.Time
}

// Captured identity and execution keep definition edits and lost replies from changing pending work.
type RunRequest struct {
	execution    Execution
	manualRecord *RunRecord
	occurrenceID occurrenceIdentity
	sessionID    string
	runID        string
	invocation   capability.Invocation
}

func (s Schedule) RecordRun(ranAt time.Time) (RunRecord, error) {
	value := RunRecord{scheduleID: s.id, ranAt: canonicalTime(ranAt)}
	if err := value.Validate(); err != nil {
		return RunRecord{}, err
	}
	if value.ranAt.Before(s.createdAt) {
		return RunRecord{}, errors.New("schedule: recorded run precedes creation")
	}
	return value, nil
}

// Delayed acknowledgements must preserve the latest accepted time while advancing this definition once.
func (s Schedule) AcceptRun(ranAt time.Time) (Schedule, error) {
	record, err := s.RecordRun(ranAt)
	if err != nil {
		return Schedule{}, err
	}
	next, err := s.advanceRevision()
	if err != nil {
		return Schedule{}, err
	}
	if record.ranAt.After(next.lastRunAt) {
		next.lastRunAt = record.ranAt
	}
	return next, nil
}

func (r RunRecord) Validate() error {
	if _, err := parseScheduleID(r.scheduleID.String()); err != nil {
		return fmt.Errorf("schedule: recorded run: %w", err)
	}
	if r.ranAt.IsZero() {
		return errors.New("schedule: recorded run time is required")
	}
	return nil
}

func (r RunRecord) ScheduleID() string { return r.scheduleID.String() }
func (r RunRecord) RanAt() time.Time   { return r.ranAt }

func (s Schedule) ManualRun(sessionID, runID string, ranAt time.Time, invocation capability.Invocation) (RunRequest, error) {
	record, err := s.RecordRun(ranAt)
	if err != nil {
		return RunRequest{}, err
	}
	value := RunRequest{
		execution: s.Execution(), manualRecord: &record,
		sessionID: sessionID, runID: runID,
		invocation: invocation,
	}
	return value, value.Validate()
}

func (o Occurrence) RunRequest() RunRequest {
	return RunRequest{
		execution: o.execution, occurrenceID: o.id,
		sessionID: o.sessionID, runID: o.runID,
		invocation: o.invocation,
	}
}

func (r RunRequest) Validate() error {
	if err := r.invocation.Validate(); err != nil {
		return err
	}
	if err := r.execution.Validate(); err != nil {
		return err
	}
	if r.manualRecord != nil {
		if r.occurrenceID.String() != "" {
			return errors.New("schedule: run request mixes manual and occurrence identity")
		}
		if err := r.manualRecord.Validate(); err != nil {
			return err
		}
	} else if err := r.occurrenceID.Validate(); err != nil {
		return err
	}
	if err := resourceid.ValidateSession(r.sessionID); err != nil {
		return fmt.Errorf("schedule: run request: %w", err)
	}
	if err := resourceid.ValidateRun(r.runID); err != nil {
		return fmt.Errorf("schedule: run request: %w", err)
	}
	return nil
}

func (r RunRequest) ScheduleID() string {
	if r.manualRecord != nil {
		return r.manualRecord.ScheduleID()
	}
	return r.occurrenceID.scheduleID.String()
}
func (r RunRequest) Execution() Execution { return r.execution }
func (r RunRequest) ManualRecord() (RunRecord, bool) {
	return optional.Present(r.manualRecord)
}
func (r RunRequest) OccurrenceID() string { return r.occurrenceID.String() }

func (r RunRequest) Acceptance() (Acceptance, bool) {
	if r.manualRecord != nil {
		return Acceptance{}, false
	}
	return Acceptance{occurrenceID: r.occurrenceID, runID: r.runID}, true
}
func (r RunRequest) SessionID() string                 { return r.sessionID }
func (r RunRequest) RunID() string                     { return r.runID }
func (r RunRequest) Invocation() capability.Invocation { return r.invocation }

type RunRequestSnapshot struct {
	ScheduleID   string
	Execution    ExecutionSnapshot
	OccurrenceID string
	SessionID    string
	RunID        string
	ManualRanAt  time.Time
	Invocation   capability.Invocation
}

func (r RunRequest) Snapshot() RunRequestSnapshot {
	value := RunRequestSnapshot{ScheduleID: r.ScheduleID(), Execution: r.execution.Snapshot(), OccurrenceID: r.OccurrenceID(), SessionID: r.sessionID, RunID: r.runID, Invocation: r.invocation}
	if r.manualRecord != nil {
		value.ManualRanAt = r.manualRecord.ranAt
	}
	return value
}
func RestoreRunRequest(value RunRequestSnapshot) (RunRequest, error) {
	execution, err := RestoreExecution(value.Execution)
	if err != nil {
		return RunRequest{}, err
	}
	request := RunRequest{execution: execution, sessionID: value.SessionID, runID: value.RunID, invocation: value.Invocation}
	if value.OccurrenceID != "" {
		identity, err := parseOccurrenceIdentity(value.OccurrenceID)
		if err != nil {
			return RunRequest{}, err
		}
		if identity.scheduleID.String() != value.ScheduleID || !value.ManualRanAt.IsZero() {
			return RunRequest{}, errors.New("schedule: inconsistent occurrence launch")
		}
		request.occurrenceID = identity
	} else {
		id, err := parseScheduleID(value.ScheduleID)
		if err != nil {
			return RunRequest{}, err
		}
		request.manualRecord = &RunRecord{scheduleID: id, ranAt: value.ManualRanAt}
	}
	return request, request.Validate()
}
