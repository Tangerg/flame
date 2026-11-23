package schedule

import (
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	flameidentity "github.com/Tangerg/flame/packages/go-sdk/identity"
	framecall "github.com/Tangerg/frame/call"
)

type RunRecord struct {
	scheduleID scheduleIdentity
	ranAt      time.Time
}

type RunRequest struct {
	execution  Execution
	origin     runOrigin
	sessionID  flameidentity.SessionID
	runID      flameidentity.RunID
	invocation framecall.Invocation
}

type runOrigin interface {
	schedule() scheduleIdentity
	validate() error
}

type manualOrigin struct {
	record    RunRecord
	commandID string
}

func (m manualOrigin) schedule() scheduleIdentity { return m.record.scheduleID }

func (m manualOrigin) validate() error {
	if err := m.record.validate(); err != nil {
		return err
	}
	if m.commandID == "" {
		return errors.New("schedule: manual command identity is required")
	}
	return nil
}

func (o occurrenceIdentity) schedule() scheduleIdentity { return o.scheduleID }

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

func (s Schedule) ManualRun(commandID string, sessionID flameidentity.SessionID, runID flameidentity.RunID, ranAt time.Time, invocation framecall.Invocation) (RunRequest, error) {
	record, err := s.recordRun(ranAt)
	if err != nil {
		return RunRequest{}, err
	}
	value := RunRequest{
		execution: s.execution(), origin: manualOrigin{record: record, commandID: commandID},
		sessionID: sessionID, runID: runID, invocation: invocation,
	}
	return value, value.Validate()
}

func (o Occurrence) RunRequest() RunRequest {
	return RunRequest{
		execution: o.execution, origin: o.id,
		sessionID: o.sessionID, runID: o.runID,
		invocation: o.invocation,
	}
}

func (r RunRequest) Validate() error {
	if err := extension.ValidateInvocation(r.invocation); err != nil {
		return err
	}
	if err := r.execution.validate(); err != nil {
		return err
	}
	if r.origin == nil {
		return errors.New("schedule: run request has no origin")
	}
	if err := r.origin.validate(); err != nil {
		return err
	}
	if r.sessionID.IsZero() || r.runID.IsZero() {
		return errors.New("schedule: run request identities are required")
	}
	return nil
}

func (r RunRequest) ScheduleID() string   { return r.origin.schedule().String() }
func (r RunRequest) Execution() Execution { return r.execution }

func (r RunRequest) ManualRecord() (RunRecord, bool) {
	manual, ok := r.origin.(manualOrigin)
	return manual.record, ok
}

func (r RunRequest) SessionID() flameidentity.SessionID { return r.sessionID }
func (r RunRequest) RunID() flameidentity.RunID         { return r.runID }
func (r RunRequest) Invocation() framecall.Invocation   { return r.invocation }

func (r RunRequest) CommandID() string {
	manual, _ := r.origin.(manualOrigin)
	return manual.commandID
}

type RunRequestSnapshot struct {
	ScheduleID   string                  `json:"scheduleId"`
	Execution    ExecutionSnapshot       `json:"execution"`
	OccurrenceID string                  `json:"occurrenceId"`
	SessionID    flameidentity.SessionID `json:"sessionId"`
	RunID        flameidentity.RunID     `json:"runId"`
	ManualRanAt  time.Time               `json:"manualRanAt"`
	Invocation   framecall.Invocation    `json:"invocation"`
	CommandID    string                  `json:"commandId,omitzero"`
}

func (r RunRequest) Snapshot() RunRequestSnapshot {
	value := RunRequestSnapshot{ScheduleID: r.ScheduleID(), Execution: r.execution.snapshot(), SessionID: r.sessionID, RunID: r.runID, Invocation: r.invocation}
	switch origin := r.origin.(type) {
	case manualOrigin:
		value.ManualRanAt, value.CommandID = origin.record.ranAt, origin.commandID
	case occurrenceIdentity:
		value.OccurrenceID = origin.String()
	}
	return value
}

func RestoreRunRequest(value RunRequestSnapshot) (RunRequest, error) {
	execution, err := restoreExecution(value.Execution)
	if err != nil {
		return RunRequest{}, err
	}
	request := RunRequest{execution: execution, sessionID: value.SessionID, runID: value.RunID, invocation: value.Invocation}
	if value.OccurrenceID != "" {
		identity, err := parseOccurrenceIdentity(value.OccurrenceID)
		if err != nil {
			return RunRequest{}, err
		}
		if identity.scheduleID.String() != value.ScheduleID || !value.ManualRanAt.IsZero() || value.CommandID != "" {
			return RunRequest{}, errors.New("schedule: inconsistent occurrence launch")
		}
		request.origin = identity
	} else {
		id, err := parseScheduleID(value.ScheduleID)
		if err != nil {
			return RunRequest{}, err
		}
		request.origin = manualOrigin{record: RunRecord{scheduleID: id, ranAt: value.ManualRanAt}, commandID: value.CommandID}
	}
	return request, request.Validate()
}
