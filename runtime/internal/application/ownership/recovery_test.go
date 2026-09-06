package ownership_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/application/ownership"
)

type testLease struct{ release func() }

func (t testLease) Release() { t.release() }

type testOwnership struct {
	err      error
	acquired bool
	released int
}

func (t *testOwnership) TryRecoverySweep() (ownership.Lease, bool, error) {
	if t.err != nil {
		return nil, false, t.err
	}
	if !t.acquired {
		return nil, false, nil
	}
	return testLease{release: func() { t.released++ }}, true, nil
}

func (t *testOwnership) AcquireRecoverySweep(context.Context) (ownership.Lease, error) {
	if !t.acquired {
		return nil, errors.New("contended")
	}
	return testLease{release: func() { t.released++ }}, nil
}

type runReconciler func(context.Context) (int, error)

func (r runReconciler) Reconcile(ctx context.Context) (int, error) {
	return r(ctx)
}

type goalReconciler func(context.Context) error

func (g goalReconciler) Reconcile(ctx context.Context) error { return g(ctx) }

func TestNewRecoveryRequiresCompleteComposition(t *testing.T) {
	validRuns := runReconciler(func(context.Context) (int, error) { return 0, nil })
	validGoals := goalReconciler(func(context.Context) error { return nil })
	validOwnership := &testOwnership{acquired: true}
	var typedNilOwnership *testOwnership

	tests := []struct {
		name      string
		runs      ownership.RunRecovery
		goals     ownership.GoalRecovery
		ownership ownership.RecoveryBackend
	}{
		{name: "missing Run reconciler", goals: validGoals, ownership: validOwnership},
		{name: "missing Goal reconciler", runs: validRuns, ownership: validOwnership},
		{name: "missing ownership backend", runs: validRuns, goals: validGoals},
		{name: "Run reconciler", runs: runReconciler(nil), goals: validGoals, ownership: validOwnership},
		{name: "Goal reconciler", runs: validRuns, goals: goalReconciler(nil), ownership: validOwnership},
		{name: "ownership backend", runs: validRuns, goals: validGoals, ownership: typedNilOwnership},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			coordinator, err := ownership.NewRecovery(test.runs, test.goals, test.ownership)
			if err == nil || coordinator != nil {
				t.Fatalf("NewRecovery = %#v, %v", coordinator, err)
			}
		})
	}
}

func TestCoordinatorElectsOneWinnerAndOrdersRunBeforeGoalRecovery(t *testing.T) {
	var order []string
	backend := &testOwnership{acquired: true}
	coordinator, err := ownership.NewRecovery(
		runReconciler(func(context.Context) (int, error) {
			order = append(order, "runs")
			return 1, nil
		}),
		goalReconciler(func(context.Context) error {
			order = append(order, "goals")
			return nil
		}),
		backend,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := coordinator.ReconcileStartup(t.Context()); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(order, []string{"runs", "goals"}) || backend.released != 1 {
		t.Fatalf("order=%v releases=%d", order, backend.released)
	}
}

func TestCoordinatorSkipsContendedSweepAndReleasesAfterFailure(t *testing.T) {
	backend := &testOwnership{}
	calls := 0
	coordinator, err := ownership.NewRecovery(
		runReconciler(func(context.Context) (int, error) {
			calls++
			return 0, errors.New("failed")
		}),
		goalReconciler(func(context.Context) error {
			t.Fatal("reconciled Goals after failed Run recovery")
			return nil
		}),
		backend,
	)
	if err != nil {
		t.Fatal(err)
	}
	acquired, err := coordinator.Reconcile(t.Context())
	if err != nil || acquired || calls != 0 {
		t.Fatalf("contended sweep: acquired=%t calls=%d err=%v", acquired, calls, err)
	}
	backend.acquired = true
	acquired, err = coordinator.Reconcile(t.Context())
	if !acquired || err == nil || backend.released != 1 {
		t.Fatalf("failed sweep: acquired=%t releases=%d err=%v", acquired, backend.released, err)
	}
}

func TestRecoveryReportsOwnershipFailureBeforeReconciliation(t *testing.T) {
	cause := errors.New("lock storage failed")
	coordinator, err := ownership.NewRecovery(
		runReconciler(func(context.Context) (int, error) { t.Fatal("reconciled without ownership"); return 0, nil }),
		goalReconciler(func(context.Context) error {
			t.Fatal("reconciled Goals without ownership")
			return nil
		}),
		&testOwnership{err: cause},
	)
	if err != nil {
		t.Fatal(err)
	}
	acquired, err := coordinator.Reconcile(t.Context())
	if acquired || !errors.Is(err, cause) {
		t.Fatalf("Reconcile = (%t, %v), want ownership cause", acquired, err)
	}
}

func TestRecoveryWorkerReportsFailuresAndResumes(t *testing.T) {
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&output, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	ctx, cancel := context.WithTimeout(t.Context(), 8*time.Second)
	defer cancel()
	backend := &testOwnership{acquired: true}
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
		backend,
	)
	if err != nil {
		t.Fatal(err)
	}

	coordinator.RunWorker(ctx)

	if attempts != 5 || backend.released != 5 || goalSweeps != 1 {
		t.Fatalf("attempts=%d releases=%d Goal sweeps=%d, want 5, 5, 1", attempts, backend.released, goalSweeps)
	}
	if got := output.String(); strings.Count(got, "level=ERROR") != 2 || strings.Count(got, cause.Error()) != 2 {
		t.Fatalf("diagnostics = %q, want one error per outage and none for shutdown", got)
	}
}
