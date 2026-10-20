package advisorylock

import (
	"errors"
	"sync"
	"testing"
)

func TestSettledReleaseRetainsFailureAcrossConcurrentCallers(t *testing.T) {
	failure := errors.New("descriptor close failed")
	calls := 0
	lease := newLease(func() (bool, error) {
		calls++
		return true, failure
	})
	var callers sync.WaitGroup
	for range 8 {
		callers.Go(func() {
			if err := lease.Release(); !errors.Is(err, failure) {
				t.Errorf("release = %v, want settled cleanup failure", err)
			}
		})
	}
	callers.Wait()
	if calls != 1 {
		t.Fatalf("cleanup calls = %d, want one", calls)
	}
}

func TestUnsettledReleaseRemainsRetryableUntilRetirement(t *testing.T) {
	failure := errors.New("unlock failed")
	calls := 0
	lease := newLease(func() (bool, error) {
		calls++
		if calls == 1 {
			return false, failure
		}
		return true, nil
	})
	if err := lease.Release(); !errors.Is(err, failure) {
		t.Fatalf("first release = %v, want unlock failure", err)
	}
	if err := lease.Release(); err != nil {
		t.Fatalf("retry = %v", err)
	}
	if err := lease.Release(); err != nil {
		t.Fatalf("settled release = %v", err)
	}
	if calls != 2 {
		t.Fatalf("cleanup calls = %d, want failed attempt and successful retry", calls)
	}
}
