package goals

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/dependency"
	"github.com/Tangerg/flame/runtime/internal/domain/automation/goal"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
)

// RunRecorder applies a terminal Goal-owned Run to the Session's Goal. Callers
// invoke it inside the Run's terminal transaction, so the Goal read, the
// Goal's decision and the replacement commit with the Run that caused them.
type RunRecorder struct {
	goals Store
}

// NewRunRecorder constructs terminal-Run application over the required store.
func NewRunRecorder(store Store) (*RunRecorder, error) {
	if dependency.Missing(store) {
		return nil, errors.New("goals: run recorder store is required")
	}
	return &RunRecorder{goals: store}, nil
}

// RecordRun lets the current Goal decide what the terminal Run means for it.
// The Run row owns the outcome and accounting; a Goal of another incarnation
// is history and is left untouched.
func (r *RunRecorder) RecordRun(ctx context.Context, value run.Run) error {
	existing, found, err := loadGoal(ctx, r.goals, value.SessionID())
	if err != nil {
		return err
	}
	if !found || existing.IncarnationID() != value.GoalIncarnationID() {
		return nil
	}
	next, changed, err := existing.RecordRun(value)
	if err != nil {
		return fmt.Errorf("goals: apply Goal Run: %w", err)
	}
	if !changed {
		return nil
	}
	change, err := goal.NewReplacement(existing.Version(), next)
	if err != nil {
		return fmt.Errorf("goals: prepare Goal Run replacement: %w", err)
	}
	applied, err := r.goals.Save(ctx, change)
	if err != nil {
		return err
	}
	if !applied {
		return errors.New("goals: record Goal Run lost Goal ownership")
	}
	return nil
}
