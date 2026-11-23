package goal

import (
	"errors"
	"fmt"
	"math"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/runtime/internal/domain/automation/goalref"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
)

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

func UsageOf(sessionID identity.SessionID, incarnationID goalref.IncarnationID, runs []run.Run) (Usage, error) {
	byID := make(map[identity.RunID]run.Run, len(runs))
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
