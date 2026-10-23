package ownership_test

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"

	"github.com/Tangerg/flame/runtime/internal/application/ownership"
)

type recoveryEntry struct {
	name string
	run  func(*ownership.RecoveryCoordinator, context.Context) error
}

type recoveryPhase uint8

const (
	recoveryRuns recoveryPhase = iota + 1
	recoveryGoals
)

func recoveryEntries() []recoveryEntry {
	return []recoveryEntry{
		{"survivor", func(c *ownership.RecoveryCoordinator, ctx context.Context) error {
			_, err := c.Reconcile(ctx)
			return err
		}},
		{"startup", (*ownership.RecoveryCoordinator).ReconcileStartup},
	}
}

func TestRecoveryRejectsCanceledAdmissionBeforeOwnership(t *testing.T) {
	for _, entry := range recoveryEntries() {
		t.Run(entry.name, func(t *testing.T) {
			var runCalls, goalCalls int
			coordinator, err := ownership.NewRecovery(
				runReconciler(func(context.Context) (int, error) { runCalls++; return 0, nil }),
				goalReconciler(func(context.Context) error { goalCalls++; return nil }),
			)
			if err != nil {
				t.Fatal(err)
			}
			failure := errors.New("recovery owner stopped")
			ctx, cancel := context.WithCancelCause(t.Context())
			cancel(failure)
			if err := entry.run(coordinator, ctx); !errors.Is(err, context.Canceled) || !errors.Is(err, failure) {
				t.Errorf("recovery = %v, want cancellation and cause", err)
			}
			if runCalls != 0 || goalCalls != 0 {
				t.Fatalf("Runs=%d Goals=%d, want no work", runCalls, goalCalls)
			}
		})
	}
}

func TestRecoveryCancellationStopsOnlyUnstartedStages(t *testing.T) {
	for _, entry := range recoveryEntries() {
		for _, stage := range []struct {
			name                string
			at                  recoveryPhase
			err                 error
			runCalls, goalCalls int
			complete            bool
		}{
			{name: "Run", at: recoveryRuns, runCalls: 1},
			{name: "failed Run", at: recoveryRuns, err: errors.New("run recovery failed"), runCalls: 1},
			{name: "failed Goal", at: recoveryGoals, err: errors.New("goal recovery failed"), runCalls: 1, goalCalls: 1},
			{name: "completed Goal", at: recoveryGoals, runCalls: 1, goalCalls: 1, complete: true},
		} {
			t.Run(entry.name+"/"+stage.name, func(t *testing.T) {
				failure := errors.New("recovery owner stopped")
				ctx, cancel := context.WithCancelCause(t.Context())
				defer cancel(nil)
				var runCalls, goalCalls int
				coordinator, err := ownership.NewRecovery(
					runReconciler(func(context.Context) (int, error) {
						runCalls++
						if stage.at == recoveryRuns {
							cancel(failure)
							return 0, stage.err
						}
						return 0, nil
					}),
					goalReconciler(func(context.Context) error {
						goalCalls++
						if stage.at == recoveryGoals {
							cancel(failure)
						}
						return stage.err
					}),
				)
				if err != nil {
					t.Fatal(err)
				}
				err = entry.run(coordinator, ctx)
				if stage.complete {
					if err != nil {
						t.Errorf("completed recovery = %v, want committed result", err)
					}
				} else if !errors.Is(err, context.Canceled) || !errors.Is(err, failure) || stage.err != nil && !errors.Is(err, stage.err) {
					t.Errorf("recovery = %v, want complete interrupted-stage outcome", err)
				}
				if runCalls != stage.runCalls || goalCalls != stage.goalCalls {
					t.Fatalf("Runs=%d Goals=%d, want %d, %d", runCalls, goalCalls, stage.runCalls, stage.goalCalls)
				}
			})
		}
	}
}

func TestRecoveryRequiresLifetimeBeforeOwnership(t *testing.T) {
	for _, entry := range recoveryEntries() {
		t.Run(entry.name, func(t *testing.T) {
			coordinator, err := ownership.NewRecovery(
				runReconciler(func(context.Context) (int, error) { t.Error("Run recovery has no lifetime"); return 0, nil }),
				goalReconciler(func(context.Context) error { t.Error("Goal recovery has no lifetime"); return nil }),
			)
			if err != nil {
				t.Fatal(err)
			}
			if err := entry.run(coordinator, nil); err == nil {
				t.Fatalf("missing lifetime = %v, want rejection before recovery", err)
			}
		})
	}
}

func TestStartupRecoveryCanCancelWhileAnotherSweepRuns(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		hold := make(chan struct{})
		runs := 0
		coordinator, err := ownership.NewRecovery(
			runReconciler(func(context.Context) (int, error) { runs++; <-hold; return 0, nil }),
			goalReconciler(func(context.Context) error { return nil }),
		)
		if err != nil {
			t.Fatal(err)
		}
		first := make(chan error, 1)
		go func() { first <- coordinator.ReconcileStartup(t.Context()) }()
		synctest.Wait()
		ctx, cancel := context.WithCancelCause(t.Context())
		defer cancel(nil)
		waiter := make(chan error, 1)
		go func() { waiter <- coordinator.ReconcileStartup(ctx) }()
		synctest.Wait()
		cause := errors.New("startup withdrawn")
		cancel(cause)
		if err := <-waiter; !errors.Is(err, context.Canceled) || !errors.Is(err, cause) {
			t.Fatalf("queued recovery = %v, want cancellation and cause", err)
		}
		if runs != 1 {
			t.Fatal("canceled recovery started work")
		}
		close(hold)
		if err := <-first; err != nil {
			t.Fatal(err)
		}
		if acquired, err := coordinator.Reconcile(t.Context()); !acquired || err != nil || runs != 2 {
			t.Fatalf("recovery after canceled waiter: acquired=%t calls=%d err=%v", acquired, runs, err)
		}
	})
}
