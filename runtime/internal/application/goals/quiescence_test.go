package goals_test

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/application/goals"
	"github.com/Tangerg/flame/runtime/internal/domain/automation/goal"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/testsupport/goaltest"
)

func TestGoalMutationPreservesOnlyFailuresFromQuiescedSettlement(t *testing.T) {
	storeFailure := errors.New("receipt store failed")
	failures := []struct {
		name     string
		returned func(context.Context) error
		want     error
	}{
		{name: "cancellation", returned: func(ctx context.Context) error { return fmt.Errorf("receipt settlement: %w", ctx.Err()) }},
		{name: "independent_failure", returned: func(context.Context) error { return storeFailure }, want: storeFailure},
		{name: "joined_failure", returned: func(ctx context.Context) error {
			return fmt.Errorf("receipt settlement: %w", errors.Join(ctx.Err(), storeFailure))
		}, want: storeFailure},
	}
	mutations := []struct {
		name      string
		apply     func(context.Context, *goals.Driver) error
		status    goal.Status
		objective string
	}{
		{name: "revise", status: goal.StatusActive, objective: "revised", apply: func(ctx context.Context, driver *goals.Driver) error {
			_, err := driver.UpdateObjective(ctx, "s1", "revised", run.Capabilities{})
			return err
		}},
		{name: "stop", status: goal.StatusPaused, objective: "original", apply: func(ctx context.Context, driver *goals.Driver) error {
			_, err := driver.Stop(ctx, "s1")
			return err
		}},
		{name: "clear", apply: func(ctx context.Context, driver *goals.Driver) error {
			return driver.Clear(ctx, "s1")
		}},
	}
	for _, mutation := range mutations {
		for _, failure := range failures {
			t.Run(mutation.name+"/"+failure.name, func(t *testing.T) {
				store := newMemStore()
				autonomous := &fakeRuns{t: t, store: store, hold: make(chan struct{}), script: []scriptedRun{{outcome: run.OutcomeCompleted}}}
				entered := make(chan struct{})
				var started atomic.Bool
				settle := func(ctx context.Context, _ string) error {
					if !started.CompareAndSwap(false, true) {
						return nil
					}
					close(entered)
					<-ctx.Done()
					return failure.returned(ctx)
				}
				driver, err := goals.NewDriver(store, autonomous, autonomous, &fakeSessions{}, goals.NewSessionMutations(), goaltest.Workflow{}, settle)
				if err != nil {
					t.Fatal(err)
				}
				cleanupDriver(t, driver)
				first, err := driver.Start(t.Context(), "s1", "original", testGoalModelSelection(), run.Capabilities{})
				if err != nil {
					t.Fatal(err)
				}
				select {
				case <-entered:
				case <-time.After(time.Second):
					t.Fatal("drive did not reach receipt settlement")
				}
				err = mutation.apply(t.Context(), driver)
				if !errors.Is(err, failure.want) {
					t.Fatalf("mutation error = %v, want %v", err, failure.want)
				}
				current, present, err := driver.Current(t.Context(), "s1")
				if err != nil {
					t.Fatal(err)
				}
				if present != (mutation.status != "") {
					t.Fatalf("goal present = %t, want %t", present, mutation.status != "")
				}
				if !present {
					return
				}
				if current.Status() != mutation.status || current.Objective() != mutation.objective {
					t.Fatalf("goal after mutation = status %s, objective %q", current.Status(), current.Objective())
				}
				if mutation.name == "revise" && current.IncarnationID() == first.IncarnationID() {
					t.Fatal("revised objective retained the original incarnation")
				}
			})
		}
	}
}
