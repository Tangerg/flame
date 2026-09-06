package ownership

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

const recoveryInterval = time.Second

// RecoveryBackend elects one Runtime process to perform a recovery sweep.
type RecoveryBackend interface {
	TryRecoverySweep() (Lease, bool, error)
	AcquireRecoverySweep(ctx context.Context) (Lease, error)
}

// RunRecovery reconciles abandoned Run trees and their Goal accounting.
type RunRecovery interface {
	Reconcile(ctx context.Context) (int, error)
}

// GoalRecovery reconciles Goal lifecycle after Run terminal accounting has settled.
type GoalRecovery interface {
	Reconcile(ctx context.Context) error
}

// RecoveryCoordinator is the single ordered recovery entry point shared by startup and
// survivor sweeps. The process winner always reconciles Runs before Goals.
type RecoveryCoordinator struct {
	runs      RunRecovery
	goals     GoalRecovery
	ownership RecoveryBackend
}

// NewRecovery constructs the ordered recovery use case with both reconcilers and
// the ownership backend required by every Runtime.
func NewRecovery(runs RunRecovery, goals GoalRecovery, ownership RecoveryBackend) (*RecoveryCoordinator, error) {
	if nilDependency(runs) {
		return nil, errors.New("ownership recovery: Run reconciler is required")
	}
	if nilDependency(goals) {
		return nil, errors.New("ownership recovery: Goal reconciler is required")
	}
	if nilDependency(ownership) {
		return nil, errors.New("ownership recovery: ownership backend is required")
	}
	return &RecoveryCoordinator{runs: runs, goals: goals, ownership: ownership}, nil
}

// Reconcile performs one non-blocking recovery sweep. acquired is false when
// another Runtime already owns the sweep; that process is responsible for the
// current pass.
func (c *RecoveryCoordinator) Reconcile(ctx context.Context) (acquired bool, err error) {
	lease, ok, err := c.ownership.TryRecoverySweep()
	if err != nil {
		return false, fmt.Errorf("ownership recovery: acquire sweep: %w", err)
	}
	if !ok {
		return false, nil
	}
	return true, c.reconcileOwned(ctx, lease)
}

// ReconcileStartup waits for the current recovery winner, then performs its
// own ordered pass before this Runtime begins serving requests. The second pass
// is intentional: candidates may have appeared after the prior winner's read.
func (c *RecoveryCoordinator) ReconcileStartup(ctx context.Context) error {
	lease, err := c.ownership.AcquireRecoverySweep(ctx)
	if err != nil {
		return fmt.Errorf("ownership recovery: acquire startup sweep: %w", err)
	}
	return c.reconcileOwned(ctx, lease)
}

// RunWorker continuously detects process death by attempting the same kernel
// leases held by live Run and Goal owners. A contended lease is definitive
// liveness evidence; no heartbeat or expiry clock participates. A failed sweep
// remains observable and is retried at the next interval.
func (c *RecoveryCoordinator) RunWorker(ctx context.Context) {
	ticker := time.NewTicker(recoveryInterval)
	defer ticker.Stop()
	failed := false
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_, err := c.Reconcile(ctx)
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

func (c *RecoveryCoordinator) reconcileOwned(ctx context.Context, lease Lease) error {
	defer lease.Release()
	if _, err := c.runs.Reconcile(ctx); err != nil {
		return fmt.Errorf("ownership recovery: reconcile Runs: %w", err)
	}
	if err := c.goals.Reconcile(ctx); err != nil {
		return fmt.Errorf("ownership recovery: reconcile Goals: %w", err)
	}
	return nil
}
