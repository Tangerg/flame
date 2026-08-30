package runs

import (
	"context"
	"errors"
	"testing"
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
}
