package goal

import (
	"errors"
	"fmt"
	"math"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
)

// Usage counts the terminal root Runs of one Goal incarnation and includes the
// process-owned spending of all their descendants. Runs remain the only durable
// accounting owners; Usage is derived by [UsageOf].
type Usage struct {
	Runs  int
	Cost  accounting.Cost
	Steps int
}

func (u Usage) validate() error {
	if u.Runs < 0 || u.Steps < 0 {
		return errors.New("goal: usage counts must be non-negative")
	}
	if u.Runs == 0 && (u.Steps != 0 || u.Cost != (accounting.Cost{})) {
		return errors.New("goal: empty usage carries spending")
	}
	return nil
}

// UsageOf requires complete terminal trees. Only roots carry Goal attribution;
// descendants inherit it through immutable lineage. Cost stays priced only while
// every process's cost is.
func UsageOf(sessionID, incarnationID string, runs []run.Run) (Usage, error) {
	byID := make(map[string]run.Run, len(runs))
	for _, value := range runs {
		if value.SessionID() != sessionID {
			return Usage{}, fmt.Errorf("%w: Run %q belongs to another Session", errRunIdentityConflict, value.ID())
		}
		if _, duplicate := byID[value.ID()]; duplicate {
			return Usage{}, fmt.Errorf("%w: Run %q appears twice", errRunIdentityConflict, value.ID())
		}
		if _, terminal := value.Outcome(); !terminal {
			return Usage{}, fmt.Errorf("goal: Run %q is not terminal", value.ID())
		}
		byID[value.ID()] = value
	}
	var used Usage
	for index, value := range runs {
		root, found := byID[value.Lineage().TreeRootID(value.ID())]
		if !found || !root.Lineage().IsRoot() || root.GoalIncarnationID() != incarnationID {
			return Usage{}, fmt.Errorf("%w: Run %q belongs to another Goal", errRunIdentityConflict, value.ID())
		}
		cost, err := value.Metrics().Cost()
		if err != nil {
			return Usage{}, fmt.Errorf("goal: Run %q cost: %w", value.ID(), err)
		}
		steps := value.Metrics().Steps()
		if steps > math.MaxInt-used.Steps {
			return Usage{}, errors.New("goal: usage counter overflow")
		}
		if index > 0 {
			cost, err = used.Cost.Add(cost)
			if err != nil {
				return Usage{}, fmt.Errorf("goal: aggregate usage cost: %w", err)
			}
		}
		if value.Lineage().IsRoot() {
			used.Runs++
		}
		used.Cost = cost
		used.Steps += steps
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
		return Goal{}, false, fmt.Errorf("%w: Run belongs to another Goal", errRunIdentityConflict)
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
