package runs

import (
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
)

// validateReductionBatch checks the complete pump-facing persistence and
// publication boundary before either side becomes observable.
func validateReductionBatch(batch reductionBatch) error {
	if len(batch.events) == 0 {
		if batch.parkCommit != nil {
			return fmt.Errorf("%w: empty batch has a park commit", errReducerInvariant)
		}
		return nil
	}

	terminalAt, err := validateReductionEvents(batch.events)
	if err != nil {
		return err
	}
	if batch.parkCommit != nil {
		return validateParkReductionBatch(batch, terminalAt)
	}
	if terminalAt < 0 {
		return nil
	}
	if err := validateTerminalReduction(batch.events[terminalAt]); err != nil {
		return err
	}
	combined, err := combineTerminalEventCommit(batch)
	if err != nil {
		return fmt.Errorf("%w: %w", errReducerInvariant, err)
	}
	if err := combined.Validate(); err != nil {
		return fmt.Errorf("%w: %w", errReducerInvariant, err)
	}
	return nil
}

func lifecycleReductions(reductions []reduction) int {
	count := 0
	for _, reduced := range reductions {
		switch reduced.Event.(type) {
		case SegmentStarted, SegmentFinished:
			count++
		}
	}
	return count
}

func validateReductionEvents(reductions []reduction) (terminalAt int, err error) {
	terminalAt = -1
	for i, reduced := range reductions {
		if reduced.Event == nil {
			return -1, fmt.Errorf("%w: reduction[%d] has no event", errReducerInvariant, i)
		}
		if _, opening := reduced.Event.(SegmentStarted); opening {
			if i != 0 {
				return -1, fmt.Errorf("%w: reduction[%d] opens a segment mid-batch", errReducerInvariant, i)
			}
			if lifecycleReductions(reductions) > 1 {
				return -1, fmt.Errorf("%w: reduction batch both opens and ends a segment", errReducerInvariant)
			}
		}
		if reduced.Event.Terminal() {
			terminalAt = i
			if i != len(reductions)-1 {
				return -1, fmt.Errorf("%w: terminal reduction[%d] is not last", errReducerInvariant, i)
			}
		}
		if reduced.Commit == nil {
			continue
		}
		switch reduced.Commit.State {
		case StateUnchanged:
		case StateSuspend:
			return -1, fmt.Errorf("%w: reduction[%d] carries a park commit", errReducerInvariant, i)
		case StateTerminalize:
			if !reduced.Event.Terminal() {
				return -1, fmt.Errorf("%w: terminal commit at reduction[%d] has no terminal event", errReducerInvariant, i)
			}
		default:
			return -1, fmt.Errorf("%w: reduction[%d] has unknown state change %q", errReducerInvariant, i, reduced.Commit.State)
		}
	}
	return terminalAt, nil
}

func validateParkReductionBatch(batch reductionBatch, terminalAt int) error {
	for i, reduced := range batch.events {
		if reduced.Commit != nil {
			return fmt.Errorf("%w: park batch event[%d] repeats a projection commit", errReducerInvariant, i)
		}
	}
	commit := batch.parkCommit
	switch {
	case commit == nil:
		return fmt.Errorf("%w: park batch has no projection commit", errReducerInvariant)
	case commit.State != StateSuspend:
		return fmt.Errorf("%w: park batch commit does not suspend the run", errReducerInvariant)
	case commit.Run == nil || commit.Run.State() != run.Waiting:
		return fmt.Errorf("%w: park batch commit has no waiting Run", errReducerInvariant)
	case terminalAt != len(batch.events)-1:
		return fmt.Errorf("%w: park batch has no terminal boundary event", errReducerInvariant)
	}
	return nil
}

func validateTerminalReduction(reduced reduction) error {
	commit := reduced.Commit
	switch {
	case commit == nil:
		return fmt.Errorf("%w: terminal event has no projection commit", errReducerInvariant)
	case commit.State != StateTerminalize:
		return fmt.Errorf("%w: terminal event commit does not terminalize the run", errReducerInvariant)
	case commit.Run == nil || !commit.Run.State().IsTerminal():
		return fmt.Errorf("%w: terminal event commit has no terminal run", errReducerInvariant)
	case commit.GoalRun != nil && (commit.GoalRun.RunID != commit.RunID || commit.GoalRun.SessionID != commit.SessionID || commit.GoalRun.Outcome != commit.Outcome):
		return fmt.Errorf("%w: terminal event commit has an inconsistent Goal Run", errReducerInvariant)
	}
	wantState, ok := run.Running.Terminate(commit.Outcome)
	committedOutcome, terminal := commit.Run.Outcome()
	if !terminal || committedOutcome != commit.Outcome {
		return fmt.Errorf("%w: terminal event commit has an inconsistent outcome", errReducerInvariant)
	}
	if !ok || commit.Run.State() != wantState {
		return fmt.Errorf("%w: terminal event commit has an invalid lifecycle transition", errReducerInvariant)
	}
	return nil
}
