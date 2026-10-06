package goal

import (
	"errors"
	"fmt"
	"math"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
)

// Usage is the accounting accumulated across one Goal incarnation's terminal
// Runs. Each Run owns its cost and steps, so Usage is only ever folded from
// those Runs by [UsageOf]; nothing stores or advances it on its own.
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

// UsageOf folds the terminal Runs of one Goal incarnation. Cost stays priced
// only while every Run's cost is.
func UsageOf(incarnationID string, runs []run.Run) (Usage, error) {
	var used Usage
	for _, value := range runs {
		if value.GoalIncarnationID() != incarnationID {
			return Usage{}, fmt.Errorf("%w: Run %q belongs to another Goal", ErrRunIdentityConflict, value.ID())
		}
		if _, terminal := value.Outcome(); !terminal {
			return Usage{}, fmt.Errorf("goal: Run %q is not terminal", value.ID())
		}
		cost, err := value.Metrics().Cost()
		if err != nil {
			return Usage{}, fmt.Errorf("goal: Run %q cost: %w", value.ID(), err)
		}
		steps := value.Metrics().Steps()
		if used.Runs == math.MaxInt || steps > math.MaxInt-used.Steps {
			return Usage{}, errors.New("goal: usage counter overflow")
		}
		if used.Runs > 0 {
			cost, err = used.Cost.Add(cost)
			if err != nil {
				return Usage{}, fmt.Errorf("goal: aggregate usage cost: %w", err)
			}
		}
		used = Usage{Runs: used.Runs + 1, Cost: cost, Steps: used.Steps + steps}
	}
	if err := used.validate(); err != nil {
		return Usage{}, err
	}
	return used, nil
}

// RecordRun applies a terminal Run of this Goal: an active Goal pauses after a
// Run that did not complete. The boolean reports whether the Goal changed; the
// Run itself is what the Goal's usage is folded from.
func (g Goal) RecordRun(value run.Run) (Goal, bool, error) {
	if value.SessionID() != g.sessionID || value.GoalIncarnationID() != g.incarnationID.String() {
		return Goal{}, false, fmt.Errorf("%w: Run belongs to another Goal", ErrRunIdentityConflict)
	}
	outcome, terminal := value.Outcome()
	if !terminal {
		return Goal{}, false, fmt.Errorf("goal: Run %q is not terminal", value.ID())
	}
	if g.status != StatusActive || outcome == run.OutcomeCompleted {
		return g, false, nil
	}
	next, err := g.next(value.FinishedAt())
	if err != nil {
		return Goal{}, false, err
	}
	next.status = StatusPaused
	next.reason, err = newReason(StatusPaused, ReasonRunNotCompleted, string(outcome))
	if err != nil {
		return Goal{}, false, err
	}
	return next, true, next.validate()
}
