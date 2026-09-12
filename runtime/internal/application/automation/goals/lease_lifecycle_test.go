package goals_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/application/automation/goals"
	"github.com/Tangerg/flame/runtime/internal/domain/automation/goal"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
)

// Release remains part of the lease lifetime: another acquisition cannot succeed
// until the ownership backend has finished unlocking and closing its resource.
type delayedDriveRelease struct {
	held           atomic.Bool
	acquisitions   atomic.Int32
	releaseStarted chan struct{}
	finishRelease  <-chan struct{}
}

func (d *delayedDriveRelease) TryGoalDrive(string) (goals.DriveLease, bool, error) {
	if !d.held.CompareAndSwap(false, true) {
		return nil, false, nil
	}
	first := d.acquisitions.Add(1) == 1
	return testDriveLease{release: sync.OnceFunc(func() {
		if first {
			close(d.releaseStarted)
			<-d.finishRelease
		}
		d.held.Store(false)
	})}, true, nil
}

func TestDriverResumeWaitsForItsPriorLeaseRelease(t *testing.T) {
	finishRelease := make(chan struct{})
	unblock := sync.OnceFunc(func() { close(finishRelease) })
	defer unblock()
	ownership := &delayedDriveRelease{
		releaseStarted: make(chan struct{}),
		finishRelease:  finishRelease,
	}
	store := newMemStore()
	driver := mustDriver(t, store, &fakeRuns{
		t: t, store: store, startErr: errors.New("provider unavailable"),
	}, &fakeSessions{}, goals.NewSessionMutations(), ownership, testPrompt)
	cleanupDriver(t, driver)

	initial, err := driver.Start(t.Context(), "s1", "finish the task", testGoalModelSelection(), goal.UnlimitedBudget(), run.Capabilities{})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-ownership.releaseStarted:
	case <-time.After(time.Second):
		t.Fatal("paused Goal did not begin releasing its drive lease")
	}

	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if _, err := driver.Resume(ctx, "s1", run.Capabilities{}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Resume while its own lease is releasing = %v, want canceled join", err)
	}
	current, present, err := driver.Current(t.Context(), "s1")
	if err != nil || !present || current.Status() != goal.StatusPaused {
		t.Fatalf("Goal after canceled resume = status %q, present %t, error %v", current.Status(), present, err)
	}

	unblock()
	resumed, err := driver.Resume(t.Context(), "s1", run.Capabilities{})
	if err != nil {
		t.Fatalf("Resume after lease release: %v", err)
	}
	if resumed.IncarnationID() != initial.IncarnationID() || resumed.Status() != goal.StatusActive {
		t.Fatalf("resumed Goal = incarnation %q, status %q", resumed.IncarnationID(), resumed.Status())
	}
	if got := ownership.acquisitions.Load(); got != 2 {
		t.Fatalf("drive acquisitions = %d, want the initial drive and one successor", got)
	}
}
