package bootstrap

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/flame/runtime/protocol"
	"github.com/Tangerg/scope/core/chat"
)

func TestProtocolKeepsCompletedChildOutcomeWhenSiblingExhaustsAllowance(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FLAME_HOME", home)
	var calls atomic.Int32
	model := delegateRestartModel{chat.ModelFunc(func(_ context.Context, request *chat.Request) (*chat.Response, error) {
		calls.Add(1)
		for _, message := range request.Messages {
			if message.Role == chat.RoleUser && strings.Contains(message.Text(), "finish child") {
				return completedTextResponse("child completed"), nil
			}
		}
		message := chat.NewAssistantMessage(
			chat.NewToolCallPart(chat.ToolCall{ID: "delegate_a", Name: "delegate_task", Arguments: `{"summary":"A","instructions":"finish child A"}`}),
			chat.NewToolCallPart(chat.ToolCall{ID: "delegate_b", Name: "delegate_task", Arguments: `{"summary":"B","instructions":"finish child B"}`}),
		)
		return chat.NewResponse(&chat.Output{Message: &message, FinishReason: chat.FinishReasonToolCalls}, &chat.ResponseMetadata{
			Model: "claude-test", Usage: chat.Usage{InputTokens: 2, OutputTokens: 1},
		})
	})}
	ctx := t.Context()
	options := delivery.Options{RequestMeta: protocol.RequestMeta{
		ProtocolVersion: protocol.ProtocolVersion,
		ClientCapabilities: &protocol.ClientCapabilities{
			Features: map[string]protocol.FeaturePreference{protocol.FeatureSubagents: {Enabled: true}},
		},
	}}
	host, api := openProtocolRuntime(t, model)
	t.Cleanup(func() {
		if err := host.Close(); err != nil {
			t.Error(err)
		}
	})
	endpoint, err := delivery.NewEndpoint(api, delivery.EndpointConfig{Lifetime: ctx})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		endpoint.BeginShutdown()
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := endpoint.AwaitShutdown(cleanup); err != nil {
			t.Error(err)
		}
	})
	created := endpoint.Invoke(ctx, delivery.SessionsCreate, protocol.CreateSessionRequest{
		Workspace: &protocol.WorkspaceRef{Path: home}, Title: "shared child allowance",
	}, options)
	if created.Failure != nil {
		t.Fatal(created.Failure)
	}
	session := created.Value.(*protocol.Session)
	started := endpoint.Invoke(ctx, delivery.RunsStart, protocol.StartRunRequest{
		SessionID: session.ID,
		Input:     []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "delegate both tasks"}},
		Limits:    &protocol.RunLimits{MaxSteps: testsupport.Pointer(2)},
	}, options)
	if started.Failure != nil {
		t.Fatal(started.Failure)
	}
	rootRunID := started.Value.(*protocol.StartRunResponse).RunID
	streamed := make(map[string]protocol.SegmentOutcomeType)
	for value, err := range started.Events {
		if err != nil {
			t.Fatal(err)
		}
		event := value.(protocol.RunEvent)
		if event.Event.Type == protocol.StreamSegmentFinished && event.Event.Outcome != nil {
			streamed[event.RunID] = event.Event.Outcome.Type
		}
	}
	listed := endpoint.Invoke(ctx, delivery.RunsList, protocol.ListRunsRequest{
		SessionID: session.ID, IncludeDescendants: true,
	}, options)
	if listed.Failure != nil {
		t.Fatal(listed.Failure)
	}
	finished := listed.Value.(*protocol.Page[protocol.RunRef])
	if len(finished.Data) != 3 || calls.Load() != 2 {
		t.Fatalf("tree = %d Runs, %d provider calls; want 3 Runs sharing 2 calls", len(finished.Data), calls.Load())
	}
	completedChildren := 0
	for _, value := range finished.Data {
		if value.Status != protocol.RunStatusFinished || value.Outcome == nil {
			t.Fatalf("unfinished Run: %+v", value)
		}
		if protocol.RunOutcomeType(streamed[value.ID]) != value.Outcome.Type {
			t.Fatalf("Run %s stream outcome %s differs from durable %s", value.ID, streamed[value.ID], value.Outcome.Type)
		}
		if value.ID != rootRunID && value.Outcome.Type == protocol.OutcomeCompleted {
			if value.Metrics.Steps != 1 {
				t.Fatalf("completed child used %d calls, want 1", value.Metrics.Steps)
			}
			completedChildren++
			continue
		}
		if value.Outcome.Type != protocol.OutcomeMaxSteps {
			t.Fatalf("Run %s ended as %s, want maxSteps", value.ID, value.Outcome.Type)
		}
	}
	if completedChildren != 1 {
		t.Fatalf("completed children = %d, want the child that returned its final answer", completedChildren)
	}
}
