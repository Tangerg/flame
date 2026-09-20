package goal

import (
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/automation/goalref"
	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
)

// Usage is the immutable accounting value accumulated across Goal-owned Runs.
type Usage struct {
	Runs  int
	Cost  accounting.Cost
	Steps int
}

func (u Usage) validate() error {
	if u.Runs < 0 || u.Steps < 0 {
		return errors.New("goal: usage counts must be non-negative")
	}
	if err := u.Cost.Validate(); err != nil {
		return fmt.Errorf("goal: usage cost: %w", err)
	}
	if u.Runs == 0 && (u.Steps != 0 || u.Cost != (accounting.Cost{})) {
		return errors.New("goal: empty usage carries spending")
	}
	return nil
}

func (u Usage) add(record RunRecord) (Usage, error) {
	if err := u.validate(); err != nil {
		return Usage{}, err
	}
	if u.Runs == math.MaxInt || record.Steps > math.MaxInt-u.Steps {
		return Usage{}, errors.New("goal: usage counter overflow")
	}
	next := Usage{
		Runs:  u.Runs + 1,
		Cost:  record.Cost,
		Steps: u.Steps + record.Steps,
	}
	if u.Runs > 0 {
		var err error
		next.Cost, err = u.Cost.Add(record.Cost)
		if err != nil {
			return Usage{}, fmt.Errorf("goal: aggregate usage cost: %w", err)
		}
	}
	if err := next.validate(); err != nil {
		return Usage{}, err
	}
	return next, nil
}

type RunRecord struct {
	SessionID     string
	IncarnationID string
	RunID         string
	Outcome       run.Outcome
	Cost          accounting.Cost
	Steps         int
	CompletedAt   time.Time
}

func (r RunRecord) Validate() error {
	if err := validateSessionIdentity(r.SessionID); err != nil {
		return fmt.Errorf("goal: Run: %w", err)
	}
	if _, err := goalref.ParseIncarnation(r.IncarnationID); err != nil {
		return fmt.Errorf("%w: Run: %v", ErrInvalid, err)
	}
	if err := resourceid.ValidateRun(r.RunID); err != nil {
		return fmt.Errorf("%w: Run ID: %v", ErrInvalid, err)
	}
	if _, ok := run.ParseOutcome(string(r.Outcome)); !ok {
		return fmt.Errorf("goal: Run has unknown outcome %q", r.Outcome)
	}
	if err := r.Cost.Validate(); err != nil {
		return fmt.Errorf("goal: Run cost: %w", err)
	}
	if r.Steps < 0 {
		return errors.New("goal: Run steps must not be negative")
	}
	if r.CompletedAt.IsZero() {
		return errors.New("goal: Run completion time is required")
	}
	return nil
}

// Describes proves this accounting record is exactly the terminal Run it names.
// Every writer of a Goal charge derives the record from the same Run — the live
// commit, boot recovery, and a terminal plan write-set — so all of them have to
// agree on what "exactly" means. It reports the defect as a phrase, leaving each
// caller its own way of failing.
func (r RunRecord) Describes(value run.Run) error {
	if err := r.Validate(); err != nil {
		return err
	}
	// A Run that has not finished has no outcome, and a validated record always
	// names one, so the comparison below is what refuses it.
	outcome, _ := value.Outcome()
	cost, err := value.Metrics().Cost()
	if err != nil {
		return fmt.Errorf("cost: %w", err)
	}
	if r.SessionID != value.SessionID() || r.IncarnationID != value.GoalIncarnationID() ||
		r.RunID != value.ID() || r.Outcome != outcome || !r.Cost.Equal(cost) ||
		r.Steps != value.Metrics().Steps() || !r.CompletedAt.Equal(value.FinishedAt()) {
		return fmt.Errorf("differs from Run %q", value.ID())
	}
	return nil
}

// ValidateCharge proves the optional accounting record filed against value is
// exactly the charge value implies: a Goal-owned Run carries one that describes
// it, and a Run outside every Goal carries none. The same three writers that
// share Describes also share this pairing, so it means one thing for all of
// them. It reports the defect as a phrase, leaving each caller its own way of
// failing.
func ValidateCharge(value run.Run, record *RunRecord) error {
	if value.GoalIncarnationID() == "" {
		if record != nil {
			return fmt.Errorf("Run %q is outside every Goal and carries a charge", value.ID())
		}
		return nil
	}
	if record == nil {
		return fmt.Errorf("Goal-owned Run %q carries no charge", value.ID())
	}
	return record.Describes(value)
}

// RecordRun accumulates usage and pauses active Goals after unsuccessful Runs.
func (g Goal) RecordRun(record RunRecord) (Goal, error) {
	if err := record.Validate(); err != nil {
		return Goal{}, err
	}
	if record.SessionID != g.sessionID || record.IncarnationID != g.incarnationID.String() {
		return Goal{}, fmt.Errorf("%w: Run belongs to another Goal", ErrRunIdentityConflict)
	}
	next, err := g.next(record.CompletedAt)
	if err != nil {
		return Goal{}, err
	}
	next.used, err = g.used.add(record)
	if err != nil {
		return Goal{}, err
	}
	if g.status == StatusActive {
		if record.Outcome != run.OutcomeCompleted {
			next.status = StatusPaused
			next.reason, err = newReason(StatusPaused, ReasonRunNotCompleted, string(record.Outcome))

		}
		if err != nil {
			return Goal{}, err
		}
	}
	return next, next.validateInitialState()
}
