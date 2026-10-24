package ownership_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"reflect"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Tangerg/flame/runtime/internal/application/ownership"
)

type runReconciler func(context.Context) (int, error)

func (r runReconciler) Reconcile(ctx context.Context) (int, error) {
	return r(ctx)
}

type goalReconciler func(context.Context) error

func (g goalReconciler) Reconcile(ctx context.Context) error { return g(ctx) }

func TestNewRecoveryRequiresCompleteComposition(t *testing.T) {
	validRuns := runReconciler(func(context.Context) (int, error) { return 0, nil })
	validGoals := goalReconciler(func(context.Context) error { return nil })
	for _, test := range []struct {
		name  string
		runs  ownership.RunRecovery
		goals ownership.GoalRecovery
	}{
		{name: "missing Run reconciler", goals: validGoals},
		{name: "missing Goal reconciler", runs: validRuns},
		{name: "typed nil Run reconciler", runs: runReconciler(nil), goals: validGoals},
		{name: "typed nil Goal reconciler", runs: validRuns, goals: goalReconciler(nil)},
	} {
		t.Run(test.name, func(t *testing.T) {
			coordinator, err := ownership.NewRecovery(test.runs, test.goals)
			if err == nil || coordinator != nil {
				t.Fatalf("NewRecovery = %#v, %v", coordinator, err)
			}
		})
	}
}

func TestCoordinatorOrdersRunBeforeGoalRecovery(t *testing.T) {
	var order []string
	coordinator, err := ownership.NewRecovery(
		runReconciler(func(context.Context) (int, error) {
			order = append(order, "runs")
			return 1, nil
		}),
		goalReconciler(func(context.Context) error {
			order = append(order, "goals")
			return nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := coordinator.ReconcileStartup(t.Context()); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(order, []string{"runs", "goals"}) {
		t.Fatalf("order = %v", order)
	}
}

func TestCoordinatorSkipsContendedSweepAndReleasesAfterFailure(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		hold := make(chan struct{})
		calls := 0
		cause := errors.New("run recovery failed")
		coordinator, err := ownership.NewRecovery(
			runReconciler(func(context.Context) (int, error) {
				calls++
				<-hold
				return 0, cause
			}),
			goalReconciler(func(context.Context) error {
				t.Error("reconciled Goals after failed Run recovery")
				return nil
			}),
		)
		if err != nil {
			t.Fatal(err)
		}
		first := make(chan error, 1)
		go func() { first <- coordinator.ReconcileStartup(t.Context()) }()
		synctest.Wait()
		if acquired, err := coordinator.Reconcile(t.Context()); err != nil || acquired || calls != 1 {
			t.Fatalf("contended sweep: acquired=%t calls=%d err=%v", acquired, calls, err)
		}
		second := make(chan error, 1)
		go func() { second <- coordinator.ReconcileStartup(t.Context()) }()
		synctest.Wait()
		if calls != 1 {
			t.Fatal("startup sweep crossed the active sweep")
		}
		close(hold)
		if err := <-first; !errors.Is(err, cause) {
			t.Fatal(err)
		}
		if err := <-second; !errors.Is(err, cause) {
			t.Fatal(err)
		}
		if acquired, err := coordinator.Reconcile(t.Context()); !acquired || !errors.Is(err, cause) || calls != 3 {
			t.Fatalf("recovery after failure: acquired=%t calls=%d err=%v", acquired, calls, err)
		}
	})
}

func TestRecoveryWorkerReportsFailuresAndResumes(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var output bytes.Buffer
		previous := slog.Default()
		slog.SetDefault(slog.New(slog.NewTextHandler(&output, nil)))
		t.Cleanup(func() { slog.SetDefault(previous) })
		ctx, cancel := context.WithTimeout(t.Context(), 8*time.Second)
		defer cancel()
		cause := errors.New("recovery store unavailable")
		var attempts, goalSweeps int
		coordinator, err := ownership.NewRecovery(
			runReconciler(func(context.Context) (int, error) {
				attempts++
				switch attempts {
				case 3:
					return 0, nil
				case 5:
					cancel()
					return 0, ctx.Err()
				default:
					return 0, cause
				}
			}),
			goalReconciler(func(context.Context) error {
				goalSweeps++
				return nil
			}),
		)
		if err != nil {
			t.Fatal(err)
		}

		coordinator.RunWorker(ctx)

		if attempts != 5 || goalSweeps != 1 {
			t.Fatalf("attempts=%d Goal sweeps=%d, want 5, 1", attempts, goalSweeps)
		}
		if got := output.String(); strings.Count(got, "level=ERROR") != 2 || strings.Count(got, cause.Error()) != 2 {
			t.Fatalf("diagnostics = %q, want one error per outage and none for shutdown", got)
		}
	})
}
