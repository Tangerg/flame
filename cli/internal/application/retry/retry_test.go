package retry

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"
)

func TestWaitHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := Wait(ctx, time.Hour); !errors.Is(err, context.Canceled) {
		t.Fatalf("Wait error = %v", err)
	}
}

func TestWaitStopsACanceledLoopEvenWhenTheDelayHasElapsed(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	// A select between a ready timer and a closed Done channel picks either
	// arm, so one attempt proves nothing about which one it prefers.
	for range 100 {
		if err := Wait(ctx, time.Nanosecond); !errors.Is(err, context.Canceled) {
			t.Fatalf("Wait error = %v", err)
		}
	}
}

func TestBackoffBoundsAnOperationOwnedRetrySchedule(t *testing.T) {
	backoff, err := NewBackoff(100*time.Millisecond, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	for failure, want := range []time.Duration{
		100 * time.Millisecond,
		200 * time.Millisecond,
		400 * time.Millisecond,
		800 * time.Millisecond,
		1600 * time.Millisecond,
		3200 * time.Millisecond,
		5 * time.Second,
		5 * time.Second,
	} {
		if got, err := backoff.Delay(failure + 1); err != nil || got != want {
			t.Fatalf("failure %d delay = %s, want %s", failure+1, got, want)
		}
	}
}

func TestBackoffRejectsAnUnconfiguredScheduleAndAnUncountedFailure(t *testing.T) {
	t.Parallel()
	if _, err := (Backoff{}).Delay(1); !errors.Is(err, ErrInvalidBackoff) {
		t.Fatalf("zero backoff delay = %v", err)
	}
	for _, bounds := range [][2]time.Duration{{0, time.Second}, {time.Second, time.Millisecond}} {
		if _, err := NewBackoff(bounds[0], bounds[1]); !errors.Is(err, ErrInvalidBackoff) {
			t.Fatalf("NewBackoff(%s, %s) = %v", bounds[0], bounds[1], err)
		}
	}
	backoff, err := NewBackoff(time.Second, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := backoff.Delay(0); !errors.Is(err, ErrInvalidBackoff) {
		t.Fatalf("Delay(0) = %v", err)
	}
}

func TestBackoffDoublesExactlyBeforeAnOddCeiling(t *testing.T) {
	for _, test := range []struct {
		name    string
		base    time.Duration
		maximum time.Duration
		want    []time.Duration
	}{
		{name: "odd nanoseconds", base: 2, maximum: 5, want: []time.Duration{2, 4, 5, 5}},
		{name: "duration limit", base: time.Duration(math.MaxInt64 / 2), maximum: time.Duration(math.MaxInt64), want: []time.Duration{math.MaxInt64 / 2, math.MaxInt64 - 1, math.MaxInt64}},
	} {
		t.Run(test.name, func(t *testing.T) {
			backoff, err := NewBackoff(test.base, test.maximum)
			if err != nil {
				t.Fatal(err)
			}
			for i, want := range test.want {
				if got, err := backoff.Delay(i + 1); err != nil || got != want {
					t.Fatalf("Delay(%d) = (%s, %v), want %s", i+1, got, err, want)
				}
			}
		})
	}
}
