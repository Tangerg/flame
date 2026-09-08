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
	for _, delay := range []time.Duration{0, time.Nanosecond, time.Hour} {
		if err := Wait(ctx, delay); !errors.Is(err, context.Canceled) {
			t.Fatalf("Wait(%s) error = %v", delay, err)
		}
	}
}

func TestBackoffDoublesUntilTheExactCeiling(t *testing.T) {
	for _, test := range []struct {
		base, maximum time.Duration
		want          []time.Duration
	}{
		{base: 1, maximum: 3, want: []time.Duration{1, 2, 3}},
		{base: math.MaxInt64 / 2, maximum: math.MaxInt64, want: []time.Duration{math.MaxInt64 / 2, math.MaxInt64 - 1, math.MaxInt64}},
	} {
		backoff, err := NewBackoff(test.base, test.maximum)
		if err != nil {
			t.Fatal(err)
		}
		for index, want := range test.want {
			if got, err := backoff.Delay(index + 1); err != nil || got != want {
				t.Fatalf("backoff (%s, %s) failure %d = (%s, %v), want %s", test.base, test.maximum, index+1, got, err, want)
			}
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

func TestBackoffRequiresPositiveOrderedDurations(t *testing.T) {
	t.Parallel()
	for _, backoff := range []Backoff{{}, {base: time.Second}} {
		if _, err := backoff.Delay(1); !errors.Is(err, ErrInvalidBackoff) {
			t.Fatalf("invalid backoff %+v = %v", backoff, err)
		}
	}
	for _, bounds := range [][2]time.Duration{{0, time.Second}, {time.Second, time.Millisecond}} {
		if _, err := NewBackoff(bounds[0], bounds[1]); !errors.Is(err, ErrInvalidBackoff) {
			t.Fatalf("NewBackoff(%s, %s) = %v", bounds[0], bounds[1], err)
		}
	}
}
