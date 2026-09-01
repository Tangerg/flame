package schedule

import (
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
)

// RunRecord is the manual execution fact that becomes durable with the Run
// opening. The aggregate constructs it so identity and time cannot drift apart.
type RunRecord struct {
	scheduleID resourceid.ScheduleID
	ranAt      time.Time
}

// RunRequest is the immutable input for launching saved instructions. Manual
// requests carry stable Session/Run identities and one Run record but no cron
// occurrence; cron requests carry the occurrence and the same stable identities.
type RunRequest struct {
	scheduleID   resourceid.ScheduleID
	execution    Execution
	manualRecord *RunRecord
	occurrenceID occurrenceIdentity
	sessionID    string
	runID        string
}

// RecordRun forms the manual execution fact owned by the Run opening without
// changing the cron cursor. The store advances it from the current revision.
func (s Schedule) RecordRun(ranAt time.Time) (RunRecord, error) {
	if err := s.Validate(); err != nil {
		return RunRecord{}, err
	}
	value := RunRecord{scheduleID: s.id, ranAt: canonicalTime(ranAt)}
	if err := value.Validate(); err != nil {
		return RunRecord{}, err
	}
	if value.ranAt.Before(s.createdAt) {
		return RunRecord{}, errors.New("schedule: recorded run precedes creation")
	}
	return value, nil
}

// Validate rejects an incomplete manual execution fact.
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

// ManualRunRequest captures an aggregate's current instructions and the run
// fact that must commit with its Run opening. It does not claim or advance the
// cron cursor.
func ManualRunRequest(s Schedule, sessionID, runID string, ranAt time.Time) (RunRequest, error) {
	record, err := s.RecordRun(ranAt)
	if err != nil {
		return RunRequest{}, err
	}
	value := RunRequest{
		scheduleID: s.id, execution: s.Execution(), manualRecord: &record,
		sessionID: sessionID, runID: runID,
	}
	return value, value.Validate()
}

// RunRequest returns the stable launch input owned by this durable occurrence.
func (o Occurrence) RunRequest() RunRequest {
	return RunRequest{
		scheduleID: o.scheduleID, execution: o.execution, occurrenceID: o.id,
		sessionID: o.sessionID, runID: o.runID,
	}
}

// Validate rejects partial durable identity. A launch is either manual with
// one aggregate-owned Run record or occurrence-backed with every stable
// identity present.
func (r RunRequest) Validate() error {
	if _, err := parseScheduleID(r.scheduleID.String()); err != nil {
		return fmt.Errorf("schedule: run request: %w", err)
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
		if r.manualRecord.scheduleID != r.scheduleID {
			return errors.New("schedule: manual run record belongs to another Schedule")
		}
		if r.sessionID == "" || r.runID == "" {
			return errors.New("schedule: manual run request identities are required")
		}
		if _, err := resourceid.ParseSession(r.sessionID); err != nil {
			return fmt.Errorf("schedule: run request: %w", err)
		}
		if _, err := resourceid.ParseRun(r.runID); err != nil {
			return fmt.Errorf("schedule: run request: %w", err)
		}
		return nil
	}
	present := 0
	for _, value := range [...]string{r.occurrenceID.String(), r.sessionID, r.runID} {
		if value != "" {
			present++
		}
	}
	if present == 0 {
		return errors.New("schedule: run request has no execution ownership")
	}
	if present != 3 {
		return errors.New("schedule: run request has partial durable identity")
	}
	if err := r.occurrenceID.Validate(); err != nil {
		return err
	}
	if r.occurrenceID.scheduleID != r.scheduleID {
		return errors.New("schedule: run request occurrence belongs to another Schedule")
	}
	if _, err := resourceid.ParseSession(r.sessionID); err != nil {
		return fmt.Errorf("schedule: run request: %w", err)
	}
	if _, err := resourceid.ParseRun(r.runID); err != nil {
		return fmt.Errorf("schedule: run request: %w", err)
	}
	return nil
}

func (r RunRequest) ScheduleID() string   { return r.scheduleID.String() }
func (r RunRequest) Execution() Execution { return r.execution }
func (r RunRequest) ManualRecord() (RunRecord, bool) {
	if r.manualRecord == nil {
		return RunRecord{}, false
	}
	return *r.manualRecord, true
}
func (r RunRequest) OccurrenceID() string { return r.occurrenceID.String() }
func (r RunRequest) SessionID() string    { return r.sessionID }
func (r RunRequest) RunID() string        { return r.runID }
