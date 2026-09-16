package agentexec

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func TestDispatchAttemptOwnsOneExternalBoundaryFact(t *testing.T) {
	attempt := newDispatchAttempt(mustInteractionEffectID(t, "effect"))
	defer attempt.close()
	if attempt.crossedExternalBoundary() {
		t.Fatal("fresh attempt reported an external side effect")
	}
	if err := attempt.beginExternalCall(); err != nil {
		t.Fatal(err)
	}
	if err := attempt.beginExternalCall(); err != nil {
		t.Fatal(err)
	}
	if !attempt.crossedExternalBoundary() {
		t.Fatal("external boundary fact was lost after repeated calls")
	}

	wantErr := errors.New("projection failed")
	attempt.recordProjectionFailure(wantErr)
	if err := attempt.indeterminateFailure(); !errors.Is(err, wantErr) {
		t.Fatalf("indeterminate failure = %v, want projection failure", err)
	}
	if err := attempt.beginExternalCall(); !errors.Is(err, wantErr) {
		t.Fatalf("call after projection failure = %v, want projection failure", err)
	}
}

func TestDispatchAttemptReleasesModelAdmissionWithoutAnExternalCall(t *testing.T) {
	allowance, err := newInteractionAllowance(testsupport.MustRunLimits(run.LimitValues{MaxSteps: testsupport.Pointer(1)}), testDefaultSelection(), nil)
	if err != nil {
		t.Fatal(err)
	}
	attempt := newDispatchAttempt(mustInteractionEffectID(t, "rejected-preparation"))
	attempt.modelAllowance, err = allowance.acquire(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	attempt.close()
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	next, err := allowance.acquire(ctx)
	if err != nil {
		t.Fatalf("next call could not acquire allowance after rejected preparation: %v", err)
	}
	next.release()
}
