package bootstrap

import (
	"context"
	"errors"
	"slices"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

func TestSequenceClosesEveryTerminalStepInReverseOrder(t *testing.T) {
	firstErr := errors.New("first diagnostic")
	lastErr := errors.New("last diagnostic")
	var calls []int
	sequence := newShutdownSequence([]*teardownStep{
		terminalStep(func(context.Context) error {
			calls = append(calls, 1)
			return firstErr
		}),
		nil,
		terminalStep(func(context.Context) error {
			calls = append(calls, 3)
			return lastErr
		}),
	})

	settled, err := sequence.shutdown(t.Context())
	if !settled || !errors.Is(err, firstErr) || !errors.Is(err, lastErr) {
		t.Fatalf("Shutdown = (settled=%v, err=%v), want both terminal diagnostics", settled, err)
	}
	if !slices.Equal(calls, []int{3, 1}) {
		t.Fatalf("close order = %v, want [3 1]", calls)
	}
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	settled, err = sequence.shutdown(canceled)
	if !settled || !errors.Is(err, firstErr) || !errors.Is(err, lastErr) {
		t.Fatalf("completed Shutdown with canceled caller = (settled=%v, err=%v), want immutable result", settled, err)
	}
	if !slices.Equal(calls, []int{3, 1}) {
		t.Fatalf("repeated Shutdown replayed steps: %v", calls)
	}
}

func TestSequenceContinuesAfterCallerTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		started := make(chan struct{})
		release := make(chan struct{})
		dependencyClosed := make(chan struct{})
		var calls atomic.Int32
		sequence := newShutdownSequence([]*teardownStep{
			terminalStep(func(context.Context) error {
				close(dependencyClosed)
				return nil
			}),
			terminalStep(func(context.Context) error {
				calls.Add(1)
				close(started)
				<-release
				return nil
			}),
		})

		ctx, cancel := context.WithTimeout(t.Context(), time.Millisecond)
		defer cancel()
		settled, err := sequence.shutdown(ctx)
		if settled || !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("bounded Shutdown = (settled=%v, err=%v), want in-flight timeout", settled, err)
		}
		<-started
		type shutdownResult struct {
			settled bool
			err     error
		}
		joined := make(chan shutdownResult, 1)
		go func() {
			settled, err := sequence.shutdown(t.Context())
			joined <- shutdownResult{settled: settled, err: err}
		}()
		close(release)
		<-dependencyClosed

		result := <-joined
		if !result.settled || result.err != nil {
			t.Fatalf("joined Shutdown = (settled=%v, err=%v), want completed graph", result.settled, result.err)
		}
		if got := calls.Load(); got != 1 {
			t.Fatalf("blocking closer calls = %d, want 1", got)
		}
	})
}

func TestSequenceRejectsCanceledAdmissionWithoutStartingTeardown(t *testing.T) {
	var calls atomic.Int32
	sequence := newShutdownSequence([]*teardownStep{terminalStep(func(context.Context) error {
		calls.Add(1)
		return nil
	})})
	cause := errors.New("shutdown caller retired")
	ctx, cancel := context.WithCancelCause(t.Context())
	cancel(cause)
	settled, err := sequence.shutdown(ctx)
	if settled || !errors.Is(err, context.Canceled) || !errors.Is(err, cause) || calls.Load() != 0 {
		t.Fatalf("canceled Shutdown = %v, %v; calls = %d", settled, err, calls.Load())
	}
	settled, err = sequence.shutdown(t.Context())
	if !settled || err != nil || calls.Load() != 1 {
		t.Fatalf("live Shutdown = %v, %v; calls = %d", settled, err, calls.Load())
	}
}
