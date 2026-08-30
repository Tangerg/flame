package agentexec

import (
	"errors"
	"testing"
)

func TestDispatchAttemptOwnsOneExternalBoundaryFact(t *testing.T) {
	attempt := newDispatchAttempt(t.Context(), mustInteractionEffectID(t, "effect"))
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
