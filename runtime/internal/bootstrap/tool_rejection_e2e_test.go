package bootstrap

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	toolcontract "github.com/Tangerg/scope/core/tool"

	"github.com/Tangerg/flame/runtime/internal/testsupport"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
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
			script := &rejectionScript{tool: test.tool, arguments: test.arguments, finish: test.finish, planMode: test.planMode}
			model := delegateRestartModel{chat.ModelFunc(script.Call)}
			stores, runtime, ctx, home := newSessionStateE2ERuntime(t, model)
			api := runtime.api
			if err := os.WriteFile(filepath.Join(home, "input.txt"), []byte("content"), 0600); err != nil {
				t.Fatal(err)
			}
			session, err := api.CreateSession(ctx, protocol.CreateSessionRequest{Workspace: &protocol.WorkspaceRef{Path: home}, Title: "tool rejection"})
			if err != nil {
				t.Fatal(err)
			}
			if test.planMode {
				if _, err := stores.ApprovalModes.StartPlanMode(ctx, testsupport.MustSessionID(session.ID)); err != nil {
					t.Fatal(err)
				}
			}
			script.readHistory = func(ctx context.Context) ([]chat.Message, error) {
				return stores.Conversations.Read(ctx, testsupport.MustSessionID(session.ID))
			}
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
			items, err := stores.Transcript.List(ctx, testsupport.MustSessionID(session.ID))
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, item := range items {
				invocation, ok := testsupport.ToolInvocationOf(item)
				if !ok || invocation.Name != test.tool {
					continue
				}
				if test.planMode {
					arguments, err := tool.ParseArguments(test.arguments)
					if err != nil {
						t.Fatal(err)
					}
					_, rejected := invocation.Input.Rejected()
					found = !rejected && invocation.Input.Arguments().Equal(arguments)
				} else if rejected, _ := invocation.Input.Rejected(); rejected == test.arguments {
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

// rejectionScript calls the rejected tool, then a valid read, then finishes;
// every later call checks that exactly the durable rejection reached the model.
type rejectionScript struct {
	tool, arguments string
	finish          chat.FinishReason
	planMode        bool
	calls           int
	readHistory     func(context.Context) ([]chat.Message, error)
}

const planRefusalText = "plan mode is active (read-only): shell is not permitted. Continue investigating with read-only tools or request Plan approval before making changes."

func (s *rejectionScript) Call(ctx context.Context, request *chat.Request) (*chat.Response, error) {
	if len(request.Tools) == 0 {
		message := chat.NewAssistantMessage(chat.NewTextPart("title"))
		return &chat.Response{Output: &chat.Output{Message: &message, FinishReason: chat.FinishReasonStop}}, nil
	}
	s.calls++
	if s.calls > 1 {
		if err := s.verify(ctx, request); err != nil {
			return nil, err
		}
	}
	if s.calls >= 3 {
		message := chat.NewAssistantMessage(chat.NewTextPart("recovered"))
		return &chat.Response{Output: &chat.Output{Message: &message, FinishReason: chat.FinishReasonStop}}, nil
	}
	call := chat.ToolCall{ID: "rejected", Name: s.tool, Arguments: s.arguments}
	finish := chat.FinishReasonToolCalls
	if s.calls == 1 && s.finish != "" {
		finish = s.finish
	}
	if s.calls == 2 {
		call = chat.ToolCall{ID: "valid", Name: "read", Arguments: `{"path":"input.txt"}`}
	}
	message := chat.NewAssistantMessage(chat.NewToolCallPart(call))
	return &chat.Response{Output: &chat.Output{Message: &message, FinishReason: finish}}, nil
}

func (s *rejectionScript) verify(ctx context.Context, request *chat.Request) error {
	history, err := s.readHistory(ctx)
	if err != nil {
		return err
	}
	committed := rejectedResult(history)
	if committed == nil || !committed.IsError {
		return fmt.Errorf("rejection missing from durable history")
	}
	if s.planMode && !reflect.DeepEqual(committed.Output, toolcontract.NewTextOutput(planRefusalText)) {
		return fmt.Errorf("Plan refusal lost its recovery instructions: %+v", committed.Output)
	}
	matches := 0
	for _, message := range request.Messages {
		for _, part := range message.Parts {
			if part.ToolResult == nil || part.ToolResult.ID != "rejected" {
				continue
			}
			matches++
			if !reflect.DeepEqual(part.ToolResult, committed) {
				return fmt.Errorf("rejection differs from durable history")
			}
		}
	}
	if matches != 1 {
		return fmt.Errorf("model request has %d rejected results, want exactly one", matches)
	}
	return nil
}

func rejectedResult(history []chat.Message) *chat.ToolResult {
	var committed *chat.ToolResult
	for _, message := range history {
		for _, part := range message.Parts {
			if part.ToolResult != nil && part.ToolResult.ID == "rejected" {
				committed = part.ToolResult
			}
		}
	}
	return committed
}
