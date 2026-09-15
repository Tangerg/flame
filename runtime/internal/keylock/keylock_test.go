package keylock

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestAcquireExcludesOneKeyAndLeavesOthersFree(t *testing.T) {
	set := NewSet()
	release, err := set.Acquire(t.Context(), "a")
	if err != nil {
		t.Fatalf("acquire a: %v", err)
	}
	other, err := set.Acquire(t.Context(), "b")
	if err != nil {
		t.Fatalf("acquire b while a is held: %v", err)
	}
	other()

	queued := make(chan struct{})
	go func() {
		again, acquireErr := set.Acquire(t.Context(), "a")
		if acquireErr != nil {
			t.Errorf("acquire a after release: %v", acquireErr)
			close(queued)
			return
		}
		again()
		close(queued)
	}()
	select {
	case <-queued:
		t.Fatal("a second holder crossed the held key")
	case <-time.After(50 * time.Millisecond):
	}
	release()
	release()
	select {
	case <-queued:
	case <-time.After(time.Second):
		t.Fatal("release did not hand the key over")
	}
}

func TestAcquireStopsWaitingWithItsOwnContext(t *testing.T) {
	set := NewSet()
	release, err := set.Acquire(t.Context(), "a")
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	defer release()

	waiting, cancelWaiting := context.WithCancel(t.Context())
	queued := make(chan error, 1)
	go func() {
		_, acquireErr := set.Acquire(waiting, "a")
		queued <- acquireErr
	}()
	cancelWaiting()
	select {
	case acquireErr := <-queued:
		if !errors.Is(acquireErr, context.Canceled) {
			t.Fatalf("waiter error = %v, want context canceled", acquireErr)
		}
	case <-time.After(time.Second):
		t.Fatal("waiter outlived its own context")
	}
}

// TestAcquireAllOrdersOverlappingKeySets proves two callers naming the same keys
// in opposite request order cannot deadlock: the reservation order is the set's,
// not the caller's.
func TestAcquireAllOrdersOverlappingKeySets(t *testing.T) {
	set := NewSet()
	var group sync.WaitGroup
	for _, keys := range [][]string{{"b", "a", "a"}, {"a", "b"}} {
		group.Add(1)
		go func() {
			defer group.Done()
			for range 50 {
				release, err := set.AcquireAll(t.Context(), keys...)
				if err != nil {
					t.Errorf("acquire %v: %v", keys, err)
					return
				}
				release()
			}
		}()
	}
	done := make(chan struct{})
	go func() { group.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("overlapping key sets deadlocked")
	}
}

// TestAcquireAllReservesNothingWhenItCannotFinish keeps a partial reservation
// from outliving the request that abandoned it.
func TestAcquireAllReservesNothingWhenItCannotFinish(t *testing.T) {
	set := NewSet()
	held, err := set.Acquire(t.Context(), "b")
	if err != nil {
		t.Fatalf("acquire b: %v", err)
	}
	defer held()

	waiting, cancelWaiting := context.WithCancel(t.Context())
	queued := make(chan error, 1)
	go func() {
		_, acquireErr := set.AcquireAll(waiting, "a", "b")
		queued <- acquireErr
	}()
	time.Sleep(20 * time.Millisecond)
	cancelWaiting()
	if acquireErr := <-queued; !errors.Is(acquireErr, context.Canceled) {
		t.Fatalf("partial acquire error = %v, want context canceled", acquireErr)
	}
	release, err := set.Acquire(t.Context(), "a")
	if err != nil {
		t.Fatalf("a stayed reserved after an abandoned AcquireAll: %v", err)
	}
	release()
}
