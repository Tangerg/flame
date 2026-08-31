package retry

import (
	"context"
	"errors"
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

func TestBackoffRequiresNamedImmediateOrBoundedPolicy(t *testing.T) {
	t.Parallel()
	if delay, err := ImmediateBackoff().Delay(1); err != nil || delay != 0 {
		t.Fatalf("immediate delay = (%s, %v)", delay, err)
	}
	for _, backoff := range []Backoff{{}, {mode: backoffImmediate, base: time.Second}} {
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
