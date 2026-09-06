package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/protocol"
	"github.com/Tangerg/scope/core/chat"
)

func TestProtocolPreservesToolPreparationFailuresAcrossRestart(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FLAME_HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".flame"), 0o700); err != nil {
		t.Fatal(err)
	}
	// The hook returns a valid decision containing an invalid argument rewrite.
	// Runtime rejects it before invoking the shell, after Scope admits the call.
	hooks := `{"hooks":[{"event":"PreToolUse","matcher":"shell","command":"printf '%s' '{\"decision\":\"allow\",\"rewriteArguments\":\"[]\"}'"}]}`
	if err := os.WriteFile(filepath.Join(home, ".flame", "hooks.json"), []byte(hooks), 0o600); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	var observed, recovered atomic.Pointer[[]chat.ToolResult]
	model := delegateRestartModel{chat.ModelFunc(func(_ context.Context, request *chat.Request) (*chat.Response, error) {
		call := calls.Add(1)
		if call == 3 || call == 4 {
			var results []chat.ToolResult
			for _, message := range request.Messages {
				for _, part := range message.Parts {
					if part.ToolResult == nil {
						continue
					}
					result := *part.ToolResult
					result.Output = result.Output.Clone()
					results = append(results, result)
				}
			}
			if call == 3 {
				observed.Store(&results)
			} else {
				recovered.Store(&results)
			}
		}
		message := chat.NewAssistantMessage(chat.NewTextPart("The hook prevented execution."))
		finish := chat.FinishReasonStop
		if call < 3 {
			message = chat.NewAssistantMessage(chat.NewToolCallPart(chat.ToolCall{
				ID: fmt.Sprintf("shell_prepare_%d", call), Name: "shell",
				Arguments: `{"command":"touch tool-executed","description":"Mark tool execution"}`,
			}))
			finish = chat.FinishReasonToolCalls
		}
		return chat.NewResponse(&chat.Output{Message: &message, FinishReason: finish}, &chat.ResponseMetadata{
			Model: "claude-test", Usage: chat.Usage{InputTokens: 2, OutputTokens: 1},
		})
	})}
	host, api := openProtocolRuntime(t, model)
	t.Cleanup(func() {
		if err := host.Close(); err != nil {
			t.Error(err)
		}
	})
	ctx := delivery.WithRequestMeta(t.Context(), protocol.RequestMeta{ProtocolVersion: protocol.ProtocolVersion})
	session, err := api.CreateSession(ctx, protocol.CreateSessionRequest{
		Workspace: &protocol.WorkspaceRef{Path: home}, Title: "tool preparation failure",
	})
	if err != nil {
		t.Fatal(err)
	}
	started, events, err := api.StartRun(ctx, protocol.StartRunRequest{
		SessionID: session.ID, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "Try the tool twice, then explain the error."}},
	})
	if err != nil {
		t.Fatal(err)
	}
	waitForRunEvents(t, collectRunEvents(events), "tool preparation failures")
	finished, err := api.GetRun(ctx, protocol.GetRunRequest{RunID: started.RunID})
	if err != nil || finished.Outcome == nil || finished.Outcome.Type != protocol.OutcomeCompleted || calls.Load() != 3 {
		diagnostic, _ := json.Marshal(finished)
		t.Fatalf("preparation failure stopped continuation: calls=%d, run=%s, error=%v", calls.Load(), diagnostic, err)
	}
	before := observed.Load()
	if before == nil || len(*before) != 2 || !(*before)[0].IsError || !(*before)[1].IsError {
		t.Fatalf("model did not receive both preparation failures: %+v", before)
	}
	if _, err := os.Stat(filepath.Join(home, "tool-executed")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("rejected Tool executed: %v", err)
	}
	snapshot, err := api.GetSessionSnapshot(ctx, protocol.GetSessionSnapshotRequest{SessionID: session.ID})
	if err != nil {
		t.Fatal(err)
	}
	failedTools := 0
	for _, item := range snapshot.Items {
		if item.Type != protocol.ItemTypeToolCall {
			continue
		}
		if item.Status != protocol.ItemStatusIncomplete || item.Error == nil || item.Tool == nil || item.Tool.Name != "shell" {
			t.Fatalf("preparation failure lost its Tool Item: %+v", item)
		}
		failedTools++
	}
	if failedTools != 2 {
		t.Fatalf("failed Tool Items = %d, want 2", failedTools)
	}
	if err := host.Close(); err != nil {
		t.Fatal(err)
	}
	restarted, api := openProtocolRuntime(t, model)
	t.Cleanup(func() {
		if err := restarted.Close(); err != nil {
			t.Error(err)
		}
	})
	afterRestart, err := api.GetSessionSnapshot(ctx, protocol.GetSessionSnapshotRequest{SessionID: session.ID})
	if err != nil || !reflect.DeepEqual(snapshot, afterRestart) {
		t.Fatalf("preparation failure Items changed across restart: %+v, %v", afterRestart, err)
	}
	_, events, err = api.StartRun(ctx, protocol.StartRunRequest{
		SessionID: session.ID, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "Explain the prior preparation failures."}},
	})
	if err != nil {
		t.Fatal(err)
	}
	waitForRunEvents(t, collectRunEvents(events), "recovered preparation failures")
	if calls.Load() != 4 || !reflect.DeepEqual(before, recovered.Load()) {
		prior, _ := json.Marshal(before)
		after, _ := json.Marshal(recovered.Load())
		t.Fatalf("preparation results changed across restart: calls=%d, before=%s, after=%s", calls.Load(), prior, after)
	}
}
