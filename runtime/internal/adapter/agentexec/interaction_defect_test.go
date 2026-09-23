package agentexec

import (
	"testing"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"
)

// TestExecutionDefectFailsTheRunAndFinishesTheSession: an impossible state in
// one execution's projection is that Run's failure. Unwinding out of the
// session's worker would take the process — and every other Session's in-flight
// work — with it, and leave this Run with no terminal at all.
func TestExecutionDefectFailsTheRunAndFinishesTheSession(t *testing.T) {
	session := newInteractionSession(
		t.Context(),
		runs.ExecutorRef{SessionID: "ses_1", ExecutorID: "turn_1"},
		runs.RootExecutionStart{SessionID: "ses_1"},
		InteractionExecutorConfig{},
		runtimeidentity.BuildID{},
		interactionExecutionPolicy{},
		nil,
	)

	func() {
		defer session.finishOnExecutionDefect()
		panic("execution projection invariant broken")
	}()

	select {
	case <-session.lifetime.done:
	default:
		t.Fatal("a defect left the session unfinished")
	}
	var terminal *runs.SegmentEnded
	for event := range session.lifetime.events {
		if ended, ok := event.Payload.(runs.SegmentEnded); ok {
			terminal = &ended
		}
	}
	if terminal == nil {
		t.Fatal("a defect published no terminal for its own Run")
	}
	if terminal.Reason != run.OutcomeFailed {
		t.Fatalf("terminal reason = %q, want failed", terminal.Reason)
	}
}
