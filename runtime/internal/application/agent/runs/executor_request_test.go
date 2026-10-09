package runs

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestPendingExecutorRequestPreservesTheCallerCancellationCause(t *testing.T) {
	request := newExecutorRequest[string]()
	want := errors.New("request owner retired")
	ctx, cancel := context.WithCancelCause(t.Context())
	cancel(want)
	if _, err := request.await(ctx); !errors.Is(err, want) {
		t.Fatalf("executor request error = %v, want owner cause", err)
	}
	if request.claim() {
		t.Fatal("canceled executor request remained claimable")
	}
	other, stop := context.WithCancelCause(t.Context())
	stop(errors.New("another wait was canceled"))
	if _, err := request.await(other); !errors.Is(err, want) {
		t.Fatalf("withdrawn request lost its original cause: %v", err)
	}
}

func TestCompletedExecutorRequestRetainsItsDecision(t *testing.T) {
	for _, want := range []executorRequestResult[string]{
		{value: "reserved child"},
		{err: errors.New("child reservation failed")},
	} {
		request := newExecutorRequest[string]()
		if !request.claim() {
			t.Fatal("request was not claimed")
		}
		if err := request.complete(want.value, want.err); err != nil {
			t.Fatal(err)
		}
		if value, err := request.await(t.Context()); value != want.value || !errors.Is(err, want.err) {
			t.Fatalf("first observation = %q, %v", value, err)
		}
		observed := make(chan executorRequestResult[string], 1)
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		go func() {
			value, err := request.await(ctx)
			observed <- executorRequestResult[string]{value: value, err: err}
		}()
		select {
		case result := <-observed:
			cancel()
			if result.value != want.value || !errors.Is(result.err, want.err) {
				t.Fatalf("completed request changed its decision: %+v", result)
			}
		case <-ctx.Done():
			cancel()
			t.Fatal("completed request waited for another decision")
		}
	}
}
