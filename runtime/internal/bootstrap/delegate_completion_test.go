package bootstrap

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/protocol"
	"github.com/Tangerg/scope/core/chat"
)

func TestProtocolCompletesAllDelegates(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FLAME_HOME", home)
	var calls atomic.Int32
	model := delegateRestartModel{chat.ModelFunc(func(_ context.Context, request *chat.Request) (*chat.Response, error) {
		calls.Add(1)
		for _, message := range request.Messages {
			if message.Role == chat.RoleTool {
				return completedTextResponse("all done"), nil
			}
		}
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
			Model: "claude-test", Usage: &chat.Usage{InputTokens: 2, OutputTokens: 1},
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
		Workspace: &protocol.WorkspaceRef{Path: home}, Title: "independent child completion",
	}, options)
	if created.Failure != nil {
		t.Fatal(created.Failure)
	}
	session := created.Value.(*protocol.Session)
	started := endpoint.Invoke(ctx, delivery.RunsStart, protocol.StartRunRequest{
		SessionID: session.ID,
		Input:     []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "delegate both tasks"}},
	}, options)
	if started.Failure != nil {
		t.Fatal(started.Failure)
	}
	rootRunID := started.Value.(*protocol.StartRunResponse).RunID
	streamed := make(map[string]protocol.SegmentOutcomeType)
	answers := make(map[string]int)
	for value, err := range started.Events {
		if err != nil {
			t.Fatal(err)
		}
		event := value.(protocol.RunEvent)
		if event.Event.Type == protocol.StreamItemCompleted && event.Event.Item.Type == protocol.ItemTypeAgentMessage {
			answers[event.RunID]++
		}
		if event.Event.Type == protocol.StreamSegmentFinished && event.Event.Outcome != nil {
			if answers[event.RunID] != 1 {
				t.Fatalf("Run %s ended with %d assistant answers, want one before its terminal", event.RunID, answers[event.RunID])
			}
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
	if len(finished.Data) != 3 || calls.Load() != 4 {
		t.Fatalf("tree = %d Runs, %d provider calls; want 3 Runs using 4 calls", len(finished.Data), calls.Load())
	}
	completedChildren := 0
	for _, value := range finished.Data {
		read := endpoint.Invoke(ctx, delivery.ItemsList, protocol.ListItemsRequest{Scope: protocol.ItemListScope{Type: protocol.ItemScopeRun, RunID: value.ID}}, options)
		if read.Failure != nil {
			t.Fatal(read.Failure)
		}
		items := read.Value.(*protocol.ListItemsResponse)
		want := "child completed"
		if value.ID == rootRunID {
			want = "all done"
		}
		var persistedAnswers int
		for _, item := range items.Data {
			if item.Type != protocol.ItemTypeAgentMessage {
				continue
			}
			persistedAnswers++
			if item.Status != protocol.ItemStatusCompleted || len(item.Content) != 1 || item.Content[0].Text != want {
				t.Fatalf("Run %s lost its committed answer: %+v", value.ID, item)
			}
		}
		if persistedAnswers != 1 || answers[value.ID] != 1 {
			t.Fatalf("Run %s has %d durable and %d streamed answers, want one each", value.ID, persistedAnswers, answers[value.ID])
		}
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
		if value.Outcome.Type != protocol.OutcomeCompleted {
			t.Fatalf("Run %s ended as %s, want completed", value.ID, value.Outcome.Type)
		}
	}
	if completedChildren != 2 {
		t.Fatalf("completed children = %d, want both children", completedChildren)
	}
}
