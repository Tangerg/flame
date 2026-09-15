package bootstrap

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/protocol"
	"github.com/Tangerg/scope/core/chat"
)

func TestRuntimeRejectedToolSurvivesFollowingCallsAndHistoryReads(t *testing.T) {
	for _, test := range []struct {
		name, tool, arguments string
		finish                chat.FinishReason
		planMode              bool
	}{
		{name: "schema", tool: "read", arguments: `{"shell":"bash"}`},
		{name: "malformed JSON", tool: "read", arguments: `{"path":`},
		{name: "non-object", tool: "read", arguments: `[]`},
		{name: "unknown", tool: "unavailable", arguments: `{}`},
		{name: "truncated", tool: "read", arguments: `{"path":"unused"}`, finish: chat.FinishReasonLength},
		{name: "delegate input", tool: "delegate_task", arguments: `{"unexpected":true}`},
		{name: "Plan refusal", tool: "shell", arguments: `{"command":"printf denied > denied.txt","description":"Write denied file"}`, planMode: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			var readHistory func(context.Context) ([]chat.Message, error)
			model := delegateRestartModel{chat.ModelFunc(func(ctx context.Context, request *chat.Request) (*chat.Response, error) {
				if len(request.Tools) == 0 {
					message := chat.NewAssistantMessage(chat.NewTextPart("title"))
					return &chat.Response{Output: &chat.Output{Message: &message, FinishReason: chat.FinishReasonStop}}, nil
				}
				calls++
				if calls > 1 {
					history, err := readHistory(ctx)
					if err != nil {
						return nil, err
					}
					var committed *chat.ToolResult
					for _, message := range history {
						for _, part := range message.Parts {
							if part.ToolResult != nil && part.ToolResult.ID == "rejected" {
								committed = part.ToolResult
							}
						}
					}
					if committed == nil || !committed.IsError {
						return nil, fmt.Errorf("rejection missing from durable history")
					}
					if test.planMode {
						want := "plan mode is active (read-only): shell is not permitted. Continue investigating with read-only tools or request Plan approval before making changes."
						if !reflect.DeepEqual(committed.Output, chat.NewTextToolOutput(want)) {
							return nil, fmt.Errorf("Plan refusal lost its recovery instructions: %+v", committed.Output)
						}
					}
					matches := 0
					for _, message := range request.Messages {
						for _, part := range message.Parts {
							if part.ToolResult != nil && part.ToolResult.ID == "rejected" {
								matches++
								if !reflect.DeepEqual(part.ToolResult, committed) {
									return nil, fmt.Errorf("rejection differs from durable history")
								}
							}
						}
					}
					if matches != 1 {
						return nil, fmt.Errorf("model request has %d rejected results, want exactly one", matches)
					}
				}
				if calls >= 3 {
					message := chat.NewAssistantMessage(chat.NewTextPart("recovered"))
					return &chat.Response{Output: &chat.Output{Message: &message, FinishReason: chat.FinishReasonStop}}, nil
				}
				call := chat.ToolCall{ID: "rejected", Name: test.tool, Arguments: test.arguments}
				finish := chat.FinishReasonToolCalls
				if calls == 1 && test.finish != "" {
					finish = test.finish
				}
				if calls == 2 {
					call = chat.ToolCall{ID: "valid", Name: "read", Arguments: `{"path":"input.txt"}`}
				}
				message := chat.NewAssistantMessage(chat.NewToolCallPart(call))
				return &chat.Response{Output: &chat.Output{Message: &message, FinishReason: finish}}, nil
			})}
			stores, api, ctx, home := newSessionStateE2ERuntime(t, model)
			if err := os.WriteFile(filepath.Join(home, "input.txt"), []byte("content"), 0600); err != nil {
				t.Fatal(err)
			}
			session, err := api.CreateSession(ctx, protocol.CreateSessionRequest{Workspace: &protocol.WorkspaceRef{Path: home}, Title: "tool rejection"})
			if err != nil {
				t.Fatal(err)
			}
			if test.planMode {
				if err := stores.PermissionModes.PutMode(ctx, session.ID, approval.SessionMode{Mode: approval.ModePlan, RestoreMode: approval.ModeBalanced}); err != nil {
					t.Fatal(err)
				}
			}
			readHistory = func(ctx context.Context) ([]chat.Message, error) { return stores.ChatHistory.Read(ctx, session.ID) }
			for range 2 {
				started, events, err := api.StartRun(ctx, protocol.StartRunRequest{SessionID: session.ID, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "continue"}}})
				if err != nil {
					t.Fatal(err)
				}
				waitForRunEvents(t, collectRunEvents(events), "tool rejection")
				finished, err := api.GetRun(ctx, protocol.GetRunRequest{RunID: started.RunID})
				if err != nil {
					t.Fatal(err)
				}
				if finished.Outcome == nil || finished.Outcome.Type != protocol.OutcomeCompleted {
					t.Fatalf("Run failed after rejection: %+v", finished.Outcome)
				}
			}
			items, err := stores.Transcript.List(ctx, session.ID)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, item := range items {
				invocation, ok := item.ToolInvocation()
				if !ok || invocation.Name != test.tool {
					continue
				}
				if test.planMode {
					arguments, err := tool.ParseArguments(test.arguments)
					if err != nil {
						t.Fatal(err)
					}
					found = invocation.ArgumentsText == "" && invocation.Arguments.Equal(arguments)
				} else if invocation.ArgumentsText == test.arguments {
					found = true
				}
			}
			if !found {
				t.Fatal("rejected arguments were not preserved in durable transcript")
			}
			if test.planMode {
				if _, err := os.Stat(filepath.Join(home, "denied.txt")); !os.IsNotExist(err) {
					t.Fatalf("Plan refusal executed the write: %v", err)
				}
			}
		})
	}
}
