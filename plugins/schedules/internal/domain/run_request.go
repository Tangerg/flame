package schedule

import (
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	flameidentity "github.com/Tangerg/flame/packages/go-sdk/identity"
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
	invocation   extension.Invocation
	commandID    string
}

func (s Schedule) recordRun(ranAt time.Time) (RunRecord, error) {
	value := RunRecord{scheduleID: s.id, ranAt: canonicalTime(ranAt)}
	if err := value.validate(); err != nil {
		return RunRecord{}, err
	}
	if value.ranAt.Before(s.createdAt) {
		return RunRecord{}, errors.New("schedule: recorded run precedes creation")
	}
	return value, nil
}

// Delayed acknowledgements must preserve the latest accepted time while advancing this definition once.
func (s Schedule) AcceptRun(ranAt time.Time) (Schedule, error) {
	record, err := s.recordRun(ranAt)
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

func (r RunRecord) validate() error {
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

func (s Schedule) ManualRun(commandID, sessionID, runID string, ranAt time.Time, invocation extension.Invocation) (RunRequest, error) {
	record, err := s.recordRun(ranAt)
	if err != nil {
		return RunRequest{}, err
	}
	value := RunRequest{
		execution: s.execution(), manualRecord: &record,
		sessionID: sessionID, runID: runID,
		invocation: invocation, commandID: commandID,
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
	if err := r.execution.validate(); err != nil {
		return err
	}
	if r.manualRecord != nil {
		if r.occurrenceID.String() != "" {
			return errors.New("schedule: run request mixes manual and occurrence identity")
		}
		if err := r.manualRecord.validate(); err != nil {
			return err
		}
		if r.commandID == "" {
			return errors.New("schedule: manual command identity is required")
		}
	} else {
		if r.commandID != "" {
			return errors.New("schedule: occurrence cannot own a manual command")
		}
		if err := r.occurrenceID.validate(); err != nil {
			return err
		}
	}
	if err := flameidentity.ValidateSession(r.sessionID); err != nil {
		return fmt.Errorf("schedule: run request: %w", err)
	}
	if err := flameidentity.ValidateRun(r.runID); err != nil {
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
	if r.manualRecord == nil {
		return RunRecord{}, false
	}
	return *r.manualRecord, true
}

func (r RunRequest) SessionID() string                { return r.sessionID }
func (r RunRequest) RunID() string                    { return r.runID }
func (r RunRequest) Invocation() extension.Invocation { return r.invocation }
func (r RunRequest) CommandID() string                { return r.commandID }

type RunRequestSnapshot struct {
	ScheduleID   string
	Execution    ExecutionSnapshot
	OccurrenceID string
	SessionID    string
	RunID        string
	ManualRanAt  time.Time
	Invocation   extension.Invocation
	CommandID    string `json:"CommandID,omitzero"`
}

func (r RunRequest) Snapshot() RunRequestSnapshot {
	value := RunRequestSnapshot{ScheduleID: r.ScheduleID(), Execution: r.execution.snapshot(), OccurrenceID: r.occurrenceID.String(), SessionID: r.sessionID, RunID: r.runID, Invocation: r.invocation, CommandID: r.commandID}
	if r.manualRecord != nil {
		value.ManualRanAt = r.manualRecord.ranAt
	}
	return value
}
func RestoreRunRequest(value RunRequestSnapshot) (RunRequest, error) {
	execution, err := restoreExecution(value.Execution)
	if err != nil {
		return RunRequest{}, err
	}
	request := RunRequest{execution: execution, sessionID: value.SessionID, runID: value.RunID, invocation: value.Invocation, commandID: value.CommandID}
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
