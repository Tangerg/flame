package ownership

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"time"

	"github.com/Tangerg/flame/runtime/dependency"
	"github.com/Tangerg/flame/runtime/internal/cancellation"
)

const recoveryInterval = time.Second

// RunRecovery reconciles abandoned Run trees and their Goal accounting.
type RunRecovery interface {
	Reconcile(ctx context.Context) (int, error)
}

// GoalRecovery reconciles Goal lifecycle after Run terminal accounting has settled.
type GoalRecovery interface {
	Reconcile(ctx context.Context) error
}

// RecoveryCoordinator is the single ordered recovery entry point shared by startup and
// periodic sweeps. Each admitted sweep reconciles Runs before Goals.
type RecoveryCoordinator struct {
	runs      RunRecovery
	goals     GoalRecovery
	sweepSlot chan struct{}
}

// NewRecovery constructs the single ordered recovery owner for one Runtime.
func NewRecovery(runs RunRecovery, goals GoalRecovery) (*RecoveryCoordinator, error) {
	if dependency.Missing(runs) {
		return nil, errors.New("ownership recovery: Run reconciler is required")
	}
	if dependency.Missing(goals) {
		return nil, errors.New("ownership recovery: Goal reconciler is required")
	}
	return &RecoveryCoordinator{runs: runs, goals: goals, sweepSlot: make(chan struct{}, 1)}, nil
}

// Reconcile performs one non-blocking recovery sweep. acquired is false when
// another sweep already holds ownership and is responsible for the
// current pass.
func (c *RecoveryCoordinator) Reconcile(ctx context.Context) (acquired bool, err error) {
	if err := recoveryContextError(ctx); err != nil {
		return false, err
	}
	select {
	case c.sweepSlot <- struct{}{}:
		defer func() { <-c.sweepSlot }()
	default:
		return false, cancellation.Err(ctx)
	}
	return true, c.reconcileOwned(ctx)
}

// ReconcileStartup joins any current recovery sweep, then reconciles Runs before
// Goals before Runtime begins serving requests.
func (c *RecoveryCoordinator) ReconcileStartup(ctx context.Context) error {
	if err := recoveryContextError(ctx); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return cancellation.Err(ctx)
	case c.sweepSlot <- struct{}{}:
		defer func() { <-c.sweepSlot }()
	}
	return c.reconcileOwned(ctx)
}

// RunWorker reconciles state abandoned by the actual Run and Goal owners.
// A failed sweep remains observable and is retried at the next interval.
func (c *RecoveryCoordinator) RunWorker(ctx context.Context) {
	ticker := time.NewTicker(recoveryInterval)
	defer ticker.Stop()
	failed := false
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			err := c.sweep(ctx)
			if err == nil {
				failed = false
			} else if !failed && ctx.Err() == nil {
				// Persistent storage failures must remain visible without emitting
				// the same outage on every one-second retry.
				slog.ErrorContext(ctx, "ownership recovery: sweep failed", "error", err)
				failed = true
			}
		}
	}
}

// sweep bounds one reconciliation. A defect in a sweep is that sweep's failure,
// carrying its stack so the worker's own outage reporting decides how loudly to
// repeat it; a panic leaving this goroutine would end the process and every
// Session in it.
func (c *RecoveryCoordinator) sweep(ctx context.Context) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("ownership recovery: sweep panicked: %v\n%s", recovered, debug.Stack())
		}
	}()
	_, err = c.Reconcile(ctx)
	return err
}

func (c *RecoveryCoordinator) reconcileOwned(ctx context.Context) error {
	if err := cancellation.Err(ctx); err != nil {
		return err
	}
	if _, err := c.runs.Reconcile(ctx); err != nil {
		return fmt.Errorf("ownership recovery: reconcile Runs: %w", errors.Join(err, cancellation.Err(ctx)))
	}
	if err := cancellation.Err(ctx); err != nil {
		return err
	}
	if err := c.goals.Reconcile(ctx); err != nil {
		return fmt.Errorf("ownership recovery: reconcile Goals: %w", errors.Join(err, cancellation.Err(ctx)))
	}
	return nil
}

func recoveryContextError(ctx context.Context) error {
	if dependency.Missing(ctx) {
		return errors.New("ownership recovery: context is required")
	}
	return cancellation.Err(ctx)
}
