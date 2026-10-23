package goals_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"

	"github.com/Tangerg/flame/runtime/internal/application/goals"
	"github.com/Tangerg/flame/runtime/internal/domain/automation/goal"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
)

type committedGoalStore struct {
	goals.Store
	committed chan struct{}
	release   <-chan struct{}
	once      sync.Once
}

func (s *committedGoalStore) Save(ctx context.Context, replacement goal.Replacement) (bool, error) {
	applied, err := s.Store.Save(ctx, replacement)
	if applied && err == nil {
		s.once.Do(func() { close(s.committed); <-s.release })
	}
	return applied, err
}

func TestRecoveryJoinsTheCommandBoundaryBeforeDecidingDriveLiveness(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		base := newMemStore()
		release := make(chan struct{})
		unblock := sync.OnceFunc(func() { close(release) })
		defer unblock()
		store := &committedGoalStore{Store: base, committed: make(chan struct{}), release: release}
		fake := &fakeRuns{t: t, store: base, script: []scriptedRun{{}}, hold: make(chan struct{}), started: make(chan struct{}, 1)}
		driver := mustDriver(t, store, fake, &fakeSessions{}, goals.NewSessionMutations(), testPrompt)
		cleanupDriver(t, driver)
		started := make(chan error, 1)
		go func() {
			_, err := driver.Start(t.Context(), "s1", "continue", testGoalModelSelection(), run.Capabilities{})
			started <- err
		}()
		<-store.committed
		ctx, cancel := context.WithCancelCause(t.Context())
		defer cancel(nil)
		canceledSweep := make(chan error, 1)
		go func() { canceledSweep <- driver.Reconcile(ctx) }()
		completedSweep := make(chan error, 1)
		go func() { completedSweep <- driver.Reconcile(t.Context()) }()
		synctest.Wait()
		select {
		case err := <-completedSweep:
			t.Fatalf("recovery crossed accepted Goal before drive launch: %v", err)
		default:
		}
		cause := errors.New("recovery withdrawn")
		cancel(cause)
		if err := <-canceledSweep; !errors.Is(err, context.Canceled) || !errors.Is(err, cause) {
			t.Fatalf("queued recovery = %v, want cancellation and cause", err)
		}
		unblock()
		if err := <-started; err != nil {
			t.Fatal(err)
		}
		if err := <-completedSweep; err != nil {
			t.Fatal(err)
		}
		<-fake.started
		current, present, err := driver.Current(t.Context(), "s1")
		if err != nil || !present || current.Status() != goal.StatusActive {
			t.Fatalf("accepted Goal after recovery = %+v, %t, %v", current, present, err)
		}
	})
}

func TestFailedClearReadPreservesTheLiveDrive(t *testing.T) {
	base := newMemStore()
	cause := errors.New("stored Goal is unavailable")
	var failed atomic.Bool
	store := boundaryGoalStore{
		Store: base,
		get: func(ctx context.Context, sessionID string) (goal.Current, error) {
			if failed.Load() {
				return goal.Current{}, cause
			}
			return base.Get(ctx, sessionID)
		},
	}
	fake := &fakeRuns{t: t, store: base, script: []scriptedRun{{}}, hold: make(chan struct{}), started: make(chan struct{}, 1)}
	driver := mustDriver(t, store, fake, &fakeSessions{}, goals.NewSessionMutations(), testPrompt)
	cleanupDriver(t, driver)
	if _, err := driver.Start(t.Context(), "s1", "continue", testGoalModelSelection(), run.Capabilities{}); err != nil {
		t.Fatal(err)
	}
	<-fake.started
	failed.Store(true)
	if err := driver.Clear(t.Context(), "s1"); !errors.Is(err, cause) {
		t.Fatalf("Clear = %v, want storage cause", err)
	}
	failed.Store(false)
	if err := driver.Reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	current, present, err := driver.Current(t.Context(), "s1")
	if err != nil || !present || current.Status() != goal.StatusActive {
		t.Fatalf("Goal after rejected Clear = %+v, %t, %v", current, present, err)
	}
}
