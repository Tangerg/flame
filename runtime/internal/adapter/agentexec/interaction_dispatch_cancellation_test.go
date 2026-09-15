package agentexec

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"testing/synctest"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/scope/core/chat"

	agent "github.com/Tangerg/scope/agent"
)

func mustInteractionProcessID(t *testing.T, value string) agent.ProcessID {
	t.Helper()
	id, err := agent.ParseProcessID(value)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func mustInteractionEffectID(t *testing.T, value string) agent.EffectID {
	t.Helper()
	id, err := agent.ParseEffectID(value)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestInteractionDispatchWaitsForSegment(t *testing.T) {
	for _, action := range []string{"activate", "cancel", "release"} {
		t.Run(action, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				session := &interactionSession{
					lifetime: newInteractionLifetime(t.Context()),
					state:    interactionState{dispatchReady: make(chan struct{})},
				}
				defer session.lifetime.beginRelease()
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				stopped := make(chan error, 1)
				go func() { stopped <- session.awaitDispatchSegment(ctx) }()
				synctest.Wait()
				select {
				case err := <-stopped:
					t.Fatalf("dispatch crossed the unopened Segment: %v", err)
				default:
				}
				var want error
				switch action {
				case "activate":
					session.state.mu.Lock()
					close(session.state.dispatchReady)
					session.state.dispatchReady = nil
					session.state.mu.Unlock()
				case "cancel":
					cancel()
					want = context.Canceled
				case "release":
					session.lifetime.beginRelease()
					want = errInteractionReleased
				}
				if err := <-stopped; !errors.Is(err, want) {
					t.Fatalf("dispatch = %v, want %v", err, want)
				}
			})
		})
	}
}

func TestLateCancellationPreservesScopeFailure(t *testing.T) {
	for _, late := range []string{"root request", "owner deadline"} {
		t.Run(late, func(t *testing.T) {
			owner, cancel := context.WithCancelCause(t.Context())
			defer cancel(nil)
			model := chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
				response := interactionToolResponse(chat.ToolCall{ID: "unexecuted", Name: "read", Arguments: `{}`}, 1, 1)
				response.Output.FinishReason = chat.FinishReasonStop
				return response, nil
			})
			executor := newTestInteractionExecutorWithLifetime(t, owner, model)
			events := runInteractionHarness(t.Context(), t, executor, interactionTestStart(), nil)
			ends := payloadsOf[runs.SegmentEnded](events)
			if len(ends) != 1 || ends[0].Reason != run.OutcomeFailed {
				t.Fatalf("initial failure = %+v", ends)
			}
			for _, session := range executor.sessions.snapshot() {
				result, err := session.state.process.Await(t.Context())
				if err != nil || result.Termination().Cause() != agent.TerminationCauseExternalFailure {
					t.Fatalf("Scope failure = %+v, %v", result.Termination(), err)
				}
				if late == "root request" {
					if err := executor.RequestRootCancellation(t.Context(), session.ref, "late cancellation"); err != nil {
						t.Fatal(err)
					}
				} else {
					cancel(context.DeadlineExceeded)
				}
				end, err := session.segmentEnd(result)
				if err != nil || !reflect.DeepEqual(end, ends[0]) {
					t.Fatalf("late cancellation changed immutable result: before=%+v after=%+v error=%v", ends[0], end, err)
				}
			}
		})
	}
}
