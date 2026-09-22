package bootstrap

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/protocol"
	"github.com/Tangerg/scope/core/chat"
)

func TestSearchInputRejectionsRemainKnownAndDurable(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FLAME_HOME", home)
	replies := 0
	model := chat.ModelFunc(func(_ context.Context, request *chat.Request) (*chat.Response, error) {
		var results []chat.ToolResult
		for _, message := range request.Messages {
			for _, part := range message.Parts {
				if part.ToolResult != nil {
					results = append(results, *part.ToolResult)
				}
			}
		}
		if len(results) > 0 {
			replies++
			if len(results) != 3 {
				return nil, fmt.Errorf("search rejections = %+v", results)
			}
			for _, result := range results {
				if !result.IsError {
					return nil, fmt.Errorf("invalid search input executed: %+v", result)
				}
			}
			return completedTextResponse("correct the search arguments"), nil
		}
		message := chat.NewAssistantMessage(
			chat.NewToolCallPart(chat.ToolCall{ID: "bad_glob", Name: "glob", Arguments: `{"pattern":"**/*.go","max_results":1e1}`}),
			chat.NewToolCallPart(chat.ToolCall{ID: "bad_grep", Name: "grep", Arguments: `{"pattern":"needle","max_results":10.0}`}),
			chat.NewToolCallPart(chat.ToolCall{ID: "bad_discovery", Name: "search_tools", Arguments: `{"query":"memory","limit":1e1}`}),
		)
		return chat.NewResponse(&chat.Output{Message: &message, FinishReason: chat.FinishReasonToolCalls}, nil)
	})
	host, api := openProtocolRuntime(t, model)
	t.Cleanup(func() {
		if err := host.Close(); err != nil {
			t.Error(err)
		}
	})
	ctx := protocolLifecycleContext(t.Context())
	session, err := api.CreateSession(ctx, protocol.CreateSessionRequest{Workspace: &protocol.WorkspaceRef{Path: home}})
	if err != nil {
		t.Fatal(err)
	}
	started, events, err := api.StartRun(ctx, protocol.StartRunRequest{
		SessionID: session.ID, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "search the workspace"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	waitForRunEvents(t, collectRunEvents(events), "search input rejection")
	ended, err := api.GetRun(ctx, protocol.GetRunRequest{RunID: started.RunID})
	if err != nil || ended.Outcome == nil || ended.Outcome.Type != protocol.OutcomeCompleted || replies != 1 {
		t.Fatalf("Run=%+v error=%v continuations=%d", ended, err, replies)
	}
	items, err := api.ListItems(ctx, protocol.ListItemsRequest{
		Scope: protocol.ItemListScope{Type: protocol.ItemScopeRun, RunID: started.RunID},
	})
	if err != nil {
		t.Fatal(err)
	}
	refused := 0
	for _, item := range items.Data {
		if item.Type != protocol.ItemTypeToolCall {
			continue
		}
		refused++
		if item.Status != protocol.ItemStatusIncomplete || item.Error == nil {
			t.Fatalf("search input refusal not durable: %+v", item)
		}
	}
	if refused != 3 {
		t.Fatalf("durable search refusals = %d, want 3", refused)
	}
}

func TestSemanticToolRefusalsKeepRootAndSiblingDurable(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FLAME_HOME", home)
	replies := 0
	model := chat.ModelFunc(func(_ context.Context, request *chat.Request) (*chat.Response, error) {
		for _, message := range request.Messages {
			if message.Role == chat.RoleUser && strings.Contains(message.Text(), "finish independent sibling") {
				return completedTextResponse("sibling done"), nil
			}
		}
		var results []chat.ToolResult
		for _, message := range request.Messages {
			for _, part := range message.Parts {
				if part.ToolResult != nil {
					results = append(results, *part.ToolResult)
				}
			}
		}
		if len(results) > 0 {
			replies++
			if len(results) != 3 || results[0].ID != "bad_shell" || !results[0].IsError || results[1].ID != "bad_path" || !results[1].IsError || results[2].IsError {
				return nil, fmt.Errorf("unexpected results: %+v", results)
			}
			return completedTextResponse("corrected"), nil
		}
		message := chat.NewAssistantMessage(
			chat.NewToolCallPart(chat.ToolCall{ID: "bad_shell", Name: "shell", Arguments: `{"command":"   ","description":"Probe"}`}),
			chat.NewToolCallPart(chat.ToolCall{ID: "bad_path", Name: "read", Arguments: `{"path":"   "}`}),
			chat.NewToolCallPart(chat.ToolCall{ID: "sibling", Name: "delegate_task", Arguments: `{"summary":"Independent","instructions":"finish independent sibling"}`}),
		)
		return chat.NewResponse(&chat.Output{Message: &message, FinishReason: chat.FinishReasonToolCalls}, nil)
	})
	host, api := openProtocolRuntime(t, model)
	t.Cleanup(func() {
		if err := host.Close(); err != nil {
			t.Error(err)
		}
	})
	ctx := delivery.WithRequestMeta(t.Context(), protocol.RequestMeta{ProtocolVersion: protocol.ProtocolVersion, ClientCapabilities: &protocol.ClientCapabilities{Features: map[string]protocol.FeaturePreference{protocol.FeatureSubagents: {Enabled: true}}}})
	session, err := api.CreateSession(ctx, protocol.CreateSessionRequest{Workspace: &protocol.WorkspaceRef{Path: home}})
	if err != nil {
		t.Fatal(err)
	}
	started, events, err := api.StartRun(ctx, protocol.StartRunRequest{SessionID: session.ID, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "validate commands and delegate"}}})
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{started.RunID: true}
	for event, err := range events {
		if err != nil {
			t.Fatal(err)
		}
		ids[event.RunID] = true
	}
	if replies != 1 || len(ids) != 2 {
		t.Fatalf("model continuation=%d runs=%v", replies, ids)
	}
	if err := host.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, reader := openProtocolRuntime(t, newReplyStub("unused"))
	defer func() {
		if err := reopened.Close(); err != nil {
			t.Error(err)
		}
	}()
	for id := range ids {
		current, err := reader.GetRun(ctx, protocol.GetRunRequest{RunID: id})
		if err != nil || current.Outcome == nil || current.Outcome.Type != protocol.OutcomeCompleted {
			t.Fatalf("run=%+v error=%v", current, err)
		}
	}
	items, err := reader.ListItems(ctx, protocol.ListItemsRequest{Scope: protocol.ItemListScope{Type: protocol.ItemScopeSession, SessionID: session.ID}})
	if err != nil {
		t.Fatal(err)
	}
	refused := 0
	for _, item := range items.Data {
		if item.Type == protocol.ItemTypeToolCall && (item.Tool.Name == "shell" || item.Tool.Name == "read") {
			refused++
			if item.Status != protocol.ItemStatusIncomplete || item.Error == nil {
				t.Fatalf("refusal not durable: %+v", item)
			}
		}
	}
	if refused != 2 {
		t.Fatalf("durable refusals=%d", refused)
	}
}
