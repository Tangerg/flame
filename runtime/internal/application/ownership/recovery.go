package ownership

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"time"

	"github.com/samber/lo"

	"github.com/Tangerg/flame/runtime/internal/telemetry"
)

const recoveryInterval = time.Second

type RunRecovery interface {
	Reconcile(ctx context.Context) (int, error)
}

type GoalRecovery interface {
	Reconcile(ctx context.Context) error
}

type RecoveryCoordinator struct {
	runs      RunRecovery
	goals     GoalRecovery
	sweepSlot chan struct{}
}

func NewRecovery(runs RunRecovery, goals GoalRecovery) (*RecoveryCoordinator, error) {
	if lo.IsNil(runs) {
		return nil, errors.New("ownership recovery: Run reconciler is required")
	}
	if lo.IsNil(goals) {
		return nil, errors.New("ownership recovery: Goal reconciler is required")
	}
	return &RecoveryCoordinator{runs: runs, goals: goals, sweepSlot: make(chan struct{}, 1)}, nil
}

func (c *RecoveryCoordinator) Reconcile(ctx context.Context) (acquired bool, err error) {
	if err := recoveryContextError(ctx); err != nil {
		return false, err
	}
	select {
	case c.sweepSlot <- struct{}{}:
		defer func() { <-c.sweepSlot }()
	default:
		return false, errors.Join(ctx.Err(), context.Cause(ctx))
	}
	return true, c.reconcileOwned(ctx)
}

func (c *RecoveryCoordinator) ReconcileStartup(ctx context.Context) error {
	if err := recoveryContextError(ctx); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return errors.Join(ctx.Err(), context.Cause(ctx))
	case c.sweepSlot <- struct{}{}:
		defer func() { <-c.sweepSlot }()
	}
	return c.reconcileOwned(ctx)
}

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
				slog.ErrorContext(ctx, "ownership recovery: sweep failed", telemetry.Error(err))
				failed = true
			}
		}
	}
}

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
	if err := errors.Join(ctx.Err(), context.Cause(ctx)); err != nil {
		return err
	}
	if _, err := c.runs.Reconcile(ctx); err != nil {
		return fmt.Errorf("ownership recovery: reconcile Runs: %w", errors.Join(err, errors.Join(ctx.Err(), context.Cause(ctx))))
	}
	if err := errors.Join(ctx.Err(), context.Cause(ctx)); err != nil {
		return err
	}
	if err := c.goals.Reconcile(ctx); err != nil {
		return fmt.Errorf("ownership recovery: reconcile Goals: %w", errors.Join(err, errors.Join(ctx.Err(), context.Cause(ctx))))
	}
	return nil
}

func recoveryContextError(ctx context.Context) error {
	if lo.IsNil(ctx) {
		return errors.New("ownership recovery: context is required")
	}
	return errors.Join(ctx.Err(), context.Cause(ctx))
}
