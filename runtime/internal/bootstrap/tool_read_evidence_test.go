package bootstrap

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/scope/core/chat"
)

func TestRuntimeMutationRequiresNewReadEvidenceAndKeepsRefusalDurable(t *testing.T) {
	script := &readEvidenceScript{steps: []readEvidenceStep{
		{call: chat.ToolCall{ID: "directory_edit", Name: "edit", Arguments: `{"path":"folder","old_string":"before","new_string":"applied"}`}, refusal: "unsupported file mode"},
		{call: chat.ToolCall{ID: "read_before", Name: "read", Arguments: `{"path":"notes.txt"}`}},
		{call: chat.ToolCall{ID: "first_edit", Name: "edit", Arguments: `{"path":"notes.txt","old_string":"before","new_string":"applied"}`}},
		{call: chat.ToolCall{ID: "unread_edit", Name: "edit", Arguments: `{"path":"notes.txt","old_string":"applied","new_string":"overwritten"}`}, refusal: "must read"},
		{call: chat.ToolCall{ID: "read_after", Name: "read", Arguments: `{"path":"notes.txt"}`}},
		{call: chat.ToolCall{ID: "second_edit", Name: "edit", Arguments: `{"path":"notes.txt","old_string":"applied","new_string":"finished"}`}},
	}}
	model := delegateRestartModel{chat.ModelFunc(script.Call)}
	stores, api, ctx, home := newSessionStateE2ERuntime(t, model)
	if err := stores.ApprovalModes.SetDefaultMode(ctx, approval.ModeBalanced); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, "notes.txt")
	if err := os.Mkdir(filepath.Join(home, "folder"), 0o700); err != nil {
		t.Fatal(err)
	}
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
	if err != nil || ended.Outcome == nil || ended.Outcome.Type != protocol.OutcomeCompleted || len(script.results) != len(script.steps) {
		t.Fatalf("Run=%+v error=%v results=%+v", ended, err, script.results)
	}
	if content, err := os.ReadFile(path); err != nil || string(content) != "finished\n" {
		t.Fatalf("workspace content = %q, %v", content, err)
	}
	history, err := stores.Conversations.Read(ctx, testsupport.MustSessionID(session.ID))
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
	if len(durable) != len(script.results) {
		t.Fatalf("durable results = %+v", durable)
	}
	for index, result := range durable {
		if !reflect.DeepEqual(result, script.results[index]) {
			t.Fatalf("durable result[%d] differs from model input", index)
		}
	}
}

type readEvidenceStep struct {
	call chat.ToolCall
	// refusal is the text a refused step must carry; empty steps succeed.
	refusal string
}

// readEvidenceScript issues each step once its predecessors' results arrived
// in order, failing the Run if any result differs from its step's expectation.
type readEvidenceScript struct {
	steps   []readEvidenceStep
	results []chat.ToolResult
}

func (s *readEvidenceScript) Call(_ context.Context, request *chat.Request) (*chat.Response, error) {
	if len(request.Tools) == 0 {
		return completedTextResponse("read evidence"), nil
	}
	s.results = nil
	for _, message := range request.Messages {
		for _, part := range message.Parts {
			if part.ToolResult != nil {
				s.results = append(s.results, *part.ToolResult)
			}
		}
	}
	for index, result := range s.results {
		if err := s.check(index, result); err != nil {
			return nil, err
		}
	}
	if len(s.results) == len(s.steps) {
		return completedTextResponse("finished after reading the applied contents"), nil
	}
	message := chat.NewAssistantMessage(chat.NewToolCallPart(s.steps[len(s.results)].call))
	return chat.NewResponse(&chat.Output{Message: &message, FinishReason: chat.FinishReasonToolCalls}, nil)
}

func (s *readEvidenceScript) check(index int, result chat.ToolResult) error {
	if index >= len(s.steps) {
		return fmt.Errorf("read evidence result[%d] = %+v", index, result)
	}
	step := s.steps[index]
	if result.ID != step.call.ID || result.IsError != (step.refusal != "") {
		return fmt.Errorf("read evidence result[%d] = %+v", index, result)
	}
	if text, _ := result.Output.Text(); step.refusal != "" && !strings.Contains(text, step.refusal) {
		return fmt.Errorf("missing refusal %q: %q", step.refusal, text)
	}
	return nil
}
