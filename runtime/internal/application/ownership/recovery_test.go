package ownership_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/application/ownership"
)

type testLease struct{ release func() }

func (t testLease) Release() { t.release() }

type testOwnership struct {
	acquired bool
	released int
}

func (t *testOwnership) TryRecoverySweep() (ownership.Lease, bool) {
	if !t.acquired {
		return nil, false
	}
	return testLease{release: func() { t.released++ }}, true
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
		nil,
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
