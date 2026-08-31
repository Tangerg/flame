package ownership

import (
	"context"
	"errors"
	"fmt"
)

// RecoveryBackend elects one Runtime process to perform a recovery sweep.
type RecoveryBackend interface {
	TryRecoverySweep() (Lease, bool)
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

// NewRecovery constructs the ordered ownership recovery use case. Goals may be nil
// when autonomous Goal capability is not assembled. A nil RecoveryBackend retains
// single-process behavior for isolated assembly tests.
func NewRecovery(runs RunRecovery, goals GoalRecovery, ownership RecoveryBackend) (*RecoveryCoordinator, error) {
	if runs == nil {
		return nil, errors.New("ownership recovery: Run reconciler is required")
	}
	if ownership == nil {
		ownership = localOwnership{}
	}
	return &RecoveryCoordinator{runs: runs, goals: goals, ownership: ownership}, nil
}

type localOwnership struct{}

func (localOwnership) TryRecoverySweep() (Lease, bool) { return localLease{}, true }

func (localOwnership) AcquireRecoverySweep(context.Context) (Lease, error) {
	return localLease{}, nil
}

type localLease struct{}

func (localLease) Release() {}

// Reconcile performs one non-blocking recovery sweep. acquired is false when
// another Runtime already owns the sweep; that process is responsible for the
// current pass.
func (c *RecoveryCoordinator) Reconcile(ctx context.Context) (acquired bool, err error) {
	lease, ok := c.ownership.TryRecoverySweep()
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

func (c *RecoveryCoordinator) reconcileOwned(ctx context.Context, lease Lease) error {
	defer lease.Release()
	if _, err := c.runs.Reconcile(ctx); err != nil {
		return fmt.Errorf("ownership recovery: reconcile Runs: %w", err)
	}
	if c.goals != nil {
		if err := c.goals.Reconcile(ctx); err != nil {
			return fmt.Errorf("ownership recovery: reconcile Goals: %w", err)
		}
	}
	return nil
}
