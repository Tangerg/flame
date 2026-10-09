package bootstrap

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/protocol"
	"github.com/Tangerg/scope/core/chat"
	chathistory "github.com/Tangerg/scope/core/history"
)

func TestRuntimeMutationRequiresNewReadEvidenceAndKeepsRefusalDurable(t *testing.T) {
	calls := []chat.ToolCall{
		{ID: "read_before", Name: "read", Arguments: `{"path":"notes.txt"}`},
		{ID: "first_edit", Name: "edit", Arguments: `{"path":"notes.txt","old_string":"before","new_string":"applied"}`},
		{ID: "unread_edit", Name: "edit", Arguments: `{"path":"notes.txt","old_string":"applied","new_string":"overwritten"}`},
		{ID: "read_after", Name: "read", Arguments: `{"path":"notes.txt"}`},
		{ID: "second_edit", Name: "edit", Arguments: `{"path":"notes.txt","old_string":"applied","new_string":"finished"}`},
	}
	var results []chat.ToolResult
	model := delegateRestartModel{chat.ModelFunc(func(_ context.Context, request *chat.Request) (*chat.Response, error) {
		if len(request.Tools) == 0 {
			return completedTextResponse("read evidence"), nil
		}
		results = nil
		for _, message := range request.Messages {
			for _, part := range message.Parts {
				if part.ToolResult != nil {
					results = append(results, *part.ToolResult)
				}
			}
		}
		for index, result := range results {
			if index >= len(calls) || result.ID != calls[index].ID || result.IsError != (index == 2) {
				return nil, fmt.Errorf("read evidence result[%d] = %+v", index, result)
			}
			if index == 2 {
				text, _ := result.Output.Text()
				if !strings.Contains(text, "must read") {
					return nil, fmt.Errorf("missing fresh-read refusal: %q", text)
				}
			}
		}
		if len(results) == len(calls) {
			return completedTextResponse("finished after reading the applied contents"), nil
		}
		message := chat.NewAssistantMessage(chat.NewToolCallPart(calls[len(results)]))
		return chat.NewResponse(&chat.Output{Message: &message, FinishReason: chat.FinishReasonToolCalls}, nil)
	})}
	stores, api, ctx, home := newSessionStateE2ERuntime(t, model)
	if err := stores.ApprovalModes.SetDefaultMode(ctx, approval.ModeBalanced); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, "notes.txt")
	if err := os.WriteFile(path, []byte("before\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	session, err := api.CreateSession(ctx, protocol.CreateSessionRequest{Workspace: &protocol.WorkspaceRef{Path: home}, Title: "read evidence"})
	if err != nil {
		t.Fatal(err)
	}
	started, events, err := api.StartRun(ctx, protocol.StartRunRequest{
		SessionID: session.ID, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "update notes"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	waitForRunEvents(t, collectRunEvents(events), "mutation read evidence")
	ended, err := api.GetRun(ctx, protocol.GetRunRequest{RunID: started.RunID})
	if err != nil || ended.Outcome == nil || ended.Outcome.Type != protocol.OutcomeCompleted || len(results) != len(calls) {
		t.Fatalf("Run=%+v error=%v results=%+v", ended, err, results)
	}
	if content, err := os.ReadFile(path); err != nil || string(content) != "finished\n" {
		t.Fatalf("workspace content = %q, %v", content, err)
	}
	history, err := stores.ChatHistory.Read(ctx, chathistory.ConversationID(session.ID))
	if err != nil {
		t.Fatal(err)
	}
	var durable []chat.ToolResult
	for _, message := range history {
		for _, part := range message.Parts {
			if part.ToolResult != nil {
				durable = append(durable, *part.ToolResult)
			}
		}
	}
	if len(durable) != len(results) {
		t.Fatalf("durable results = %+v", durable)
	}
	for index, result := range durable {
		if !reflect.DeepEqual(result, results[index]) {
			t.Fatalf("durable result[%d] differs from model input", index)
		}
	}
}
