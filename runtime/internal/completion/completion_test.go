package completion

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
)

func TestWaitPrefersObservableCompletionOverCancellation(t *testing.T) {
	t.Parallel()

	done := make(chan struct{})
	close(done)
	ctx, cancel := context.WithCancelCause(t.Context())
	cancel(errors.New("waiter retired after completion"))

	if err := Wait(ctx, done); err != nil {
		t.Fatalf("Wait() = %v, want completed", err)
	}
}

func TestWaitReportsCancellationWhileCompletionIsPending(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if err := Wait(ctx, make(chan struct{})); !errors.Is(err, context.Canceled) {
		t.Fatalf("Wait() = %v, want context.Canceled", err)
	}
}

func TestWaitPreservesTheOwnerCancellationCause(t *testing.T) {
	t.Parallel()

	want := errors.New("runtime owner stopped")
	ctx, cancel := context.WithCancelCause(t.Context())
	cancel(want)

	if err := Wait(ctx, make(chan struct{})); !errors.Is(err, context.Canceled) || !errors.Is(err, want) {
		t.Fatalf("Wait() = %v, want cancellation category and owner cause", err)
	}
}

func TestWaitPreservesCancellationWhileWaiting(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cause := errors.New("completion waiter retired")
		ctx, cancel := context.WithCancelCause(t.Context())
		defer cancel(nil)
		result := make(chan error, 1)
		go func() { result <- Wait(ctx, make(chan struct{})) }()
		synctest.Wait()
		cancel(cause)
		if err := <-result; !errors.Is(err, context.Canceled) || !errors.Is(err, cause) {
			t.Fatalf("Wait() = %v, want cancellation category and owner cause", err)
		}
	})
}
