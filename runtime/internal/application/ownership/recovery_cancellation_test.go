package ownership_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/application/ownership"
)

type recoveryEntry struct {
	name string
	run  func(*ownership.RecoveryCoordinator, context.Context) error
}

type recoveryPhase uint8

const (
	recoveryOwnership recoveryPhase = iota + 1
	recoveryRuns
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
			backend := &testOwnership{acquired: true}
			var runCalls, goalCalls int
			coordinator, err := ownership.NewRecovery(
				runReconciler(func(context.Context) (int, error) { runCalls++; return 0, nil }),
				goalReconciler(func(context.Context) error { goalCalls++; return nil }),
				backend,
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
			if backend.claims != 0 || backend.released != 0 || runCalls != 0 || goalCalls != 0 {
				t.Fatalf("claims=%d releases=%d Runs=%d Goals=%d, want no admission or work", backend.claims, backend.released, runCalls, goalCalls)
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
			{name: "ownership", at: recoveryOwnership},
			{name: "Run", at: recoveryRuns, runCalls: 1},
			{name: "failed Run", at: recoveryRuns, err: errors.New("run recovery failed"), runCalls: 1},
			{name: "failed Goal", at: recoveryGoals, err: errors.New("goal recovery failed"), runCalls: 1, goalCalls: 1},
			{name: "completed Goal", at: recoveryGoals, runCalls: 1, goalCalls: 1, complete: true},
		} {
			t.Run(entry.name+"/"+stage.name, func(t *testing.T) {
				failure := errors.New("recovery owner stopped")
				ctx, cancel := context.WithCancelCause(t.Context())
				defer cancel(nil)
				backend := &testOwnership{acquired: true}
				if stage.at == recoveryOwnership {
					backend.claim = func() { cancel(failure) }
				}
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
					backend,
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
				if backend.claims != 1 || backend.released != 1 || runCalls != stage.runCalls || goalCalls != stage.goalCalls {
					t.Fatalf("claims=%d releases=%d Runs=%d Goals=%d, want 1, 1, %d, %d", backend.claims, backend.released, runCalls, goalCalls, stage.runCalls, stage.goalCalls)
				}
			})
		}
	}
}

func TestRecoveryRequiresLifetimeBeforeOwnership(t *testing.T) {
	for _, entry := range recoveryEntries() {
		t.Run(entry.name, func(t *testing.T) {
			backend := &testOwnership{acquired: true}
			coordinator, err := ownership.NewRecovery(
				runReconciler(func(context.Context) (int, error) { t.Error("Run recovery has no lifetime"); return 0, nil }),
				goalReconciler(func(context.Context) error { t.Error("Goal recovery has no lifetime"); return nil }),
				backend,
			)
			if err != nil {
				t.Fatal(err)
			}
			if err := entry.run(coordinator, nil); err == nil || backend.claims != 0 {
				t.Fatalf("missing lifetime = %v, claims = %d, want rejection before ownership", err, backend.claims)
			}
		})
	}
}
