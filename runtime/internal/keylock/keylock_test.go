package keylock

import (
	"context"
	"errors"
	"sync"
	"testing"
	"testing/synctest"
)

func TestAcquireExcludesOneKeyAndLeavesOthersFree(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
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
		synctest.Wait()
		select {
		case <-queued:
			t.Fatal("a second holder crossed the held key")
		default:
		}
		release()
		release()
		<-queued
	})
}

func TestAcquireRefusesCanceledAdmissionWithoutReservingAKey(t *testing.T) {
	set := NewSet()
	cause := errors.New("key request retired")
	ctx, cancel := context.WithCancelCause(t.Context())
	cancel(cause)
	if release, err := set.Acquire(ctx, "a"); release != nil || !errors.Is(err, context.Canceled) || !errors.Is(err, cause) {
		t.Fatalf("canceled acquire = %v, %v; want no reservation and complete cancellation", release != nil, err)
	}
	release, err := set.Acquire(t.Context(), "a")
	if err != nil {
		t.Fatalf("canceled request retained a: %v", err)
	}
	release()
}

func TestAcquireStopsWaitingWithItsOwnContext(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		set := NewSet()
		release, err := set.Acquire(t.Context(), "a")
		if err != nil {
			t.Fatalf("acquire: %v", err)
		}
		defer release()

		cause := errors.New("key waiter retired")
		waiting, cancelWaiting := context.WithCancelCause(t.Context())
		defer cancelWaiting(nil)
		queued := make(chan error, 1)
		go func() {
			_, acquireErr := set.Acquire(waiting, "a")
			queued <- acquireErr
		}()
		synctest.Wait()
		cancelWaiting(cause)
		if acquireErr := <-queued; !errors.Is(acquireErr, context.Canceled) || !errors.Is(acquireErr, cause) {
			t.Fatalf("waiter error = %v, want cancellation category and owner cause", acquireErr)
		}
	})
}

func TestAcquireAllOrdersOverlappingKeySets(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		set := NewSet()
		var group sync.WaitGroup
		for _, keys := range [][]string{{"b", "a", "a"}, {"a", "b"}} {
			group.Go(func() {
				for range 50 {
					release, err := set.AcquireAll(t.Context(), keys...)
					if err != nil {
						t.Errorf("acquire %v: %v", keys, err)
						return
					}
					release()
				}
			})
		}
		group.Wait()
	})
}

func TestAcquireAllReservesNothingWhenItCannotFinish(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		set := NewSet()
		held, err := set.Acquire(t.Context(), "b")
		if err != nil {
			t.Fatalf("acquire b: %v", err)
		}
		defer held()

		cause := errors.New("multi-key waiter retired")
		waiting, cancelWaiting := context.WithCancelCause(t.Context())
		defer cancelWaiting(nil)
		queued := make(chan error, 1)
		go func() {
			_, acquireErr := set.AcquireAll(waiting, "a", "b")
			queued <- acquireErr
		}()
		synctest.Wait()
		cancelWaiting(cause)
		if acquireErr := <-queued; !errors.Is(acquireErr, context.Canceled) || !errors.Is(acquireErr, cause) {
			t.Fatalf("partial acquire error = %v, want cancellation category and owner cause", acquireErr)
		}
		release, err := set.Acquire(t.Context(), "a")
		if err != nil {
			t.Fatalf("a stayed reserved after an abandoned AcquireAll: %v", err)
		}
		release()
	})
}
