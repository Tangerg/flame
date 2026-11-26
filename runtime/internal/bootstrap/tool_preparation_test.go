package bootstrap

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/scope/core/chat"
)

func TestPlanWorkflowAuthoringAndWithdrawal(t *testing.T) {
	home := testsupport.TempDirectory(t)
	t.Setenv("FLAME_HOME", home)
	var phase atomic.Int32
	model := chat.ModelFunc(func(_ context.Context, request *chat.Request) (*chat.Response, error) {
		if len(request.Tools) == 0 {
			return completedTextResponse("title"), nil
		}
		for _, tool := range request.Tools {
			if tool.Name == "set_plan" {
				if phase.Load() == 2 {
					return nil, errors.New("withdrawn plan author is still advertised")
				}
				if phase.CompareAndSwap(0, 1) {
					message := chat.NewAssistantMessage(chat.NewToolCallPart(chat.ToolCall{ID: "author-plan", Name: tool.Name, Arguments: `{"steps":[{"description":"inspect original source","status":"in_progress"}]}`}))
					return chat.NewResponse(&chat.Output{Message: &message, FinishReason: chat.FinishReasonToolCalls}, nil)
				}
			}
		}
		if phase.Load() == 0 {
			return nil, errors.New("selected plan author not advertised")
		}
		return completedTextResponse("finish"), nil
	})
	host, api := openProtocolRuntime(t, model)
	t.Cleanup(func() {
		if err := host.Close(); err != nil {
			t.Error(err)
		}
	})
	r := &poMCPRuntime{t: t, host: host, api: api, endpoint: host.Endpoint()}
	installed := poMCPActivate(r, filepath.Join(firstPartyPackages, "plans"))
	ctx := protocolLifecycleContext(t.Context())
	session, err := api.CreateSession(ctx, protocol.CreateSessionRequest{Workspace: &protocol.WorkspaceRef{Path: home}})
	if err != nil {
		t.Fatal(err)
	}
	run := func() {
		t.Helper()
		start, events, err := api.StartRun(ctx, protocol.StartRunRequest{SessionID: session.ID, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "plan"}}})
		if err != nil {
			t.Fatal(err)
		}
		waitForRunEvents(t, collectRunEvents(events), "plan author")
		terminal, err := api.GetRun(ctx, protocol.GetRunRequest{RunID: start.RunID})
		if err != nil || terminal.Outcome == nil || terminal.Outcome.Type != protocol.OutcomeCompleted {
			t.Fatalf("run=%+v, %v", terminal, err)
		}
	}
	run()
	read := func() {
		t.Helper()
		current, err := api.GetPlan(ctx, protocol.GetPlanRequest{SessionID: session.ID})
		if err != nil || current.State == nil || current.State.Revision != 1 || len(current.State.Steps) != 1 || current.State.Steps[0].Description != "inspect original source" {
			t.Fatalf("current plan=%+v, %v", current, err)
		}
	}
	read()
	r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: installed.ID, Enabled: false}, "withdraw-plan")
	phase.Store(2)
	run()
	read()
}

func TestProtocolPreservesToolPreparationFailuresAcrossRestart(t *testing.T) {
	home := testsupport.TempDirectory(t)
	t.Setenv("FLAME_HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".flame"), 0o700); err != nil {
		t.Fatal(err)
	}
	hooks := `{"hooks":[{"event":"PreToolUse","matcher":"shell","command":"printf '%s' '{\"decision\":\"allow\",\"rewriteArguments\":\"[]\"}'"}]}`
	if err := os.WriteFile(filepath.Join(home, ".flame", "hooks.json"), []byte(hooks), 0o600); err != nil {
		t.Fatal(err)
	}
	script := &preparationFailureScript{}
	model := delegateRestartModel{chat.ModelFunc(script.Call)}
	host, api := openProtocolRuntime(t, model)
	t.Cleanup(func() {
		if err := host.Close(); err != nil {
			t.Error(err)
		}
	})
	ctx := delivery.WithRequestMeta(t.Context(), protocol.RequestMeta{ProtocolVersion: new(protocol.Version)})
	activateHookProvider(t, api)
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
	if err != nil || finished.Outcome == nil || finished.Outcome.Type != protocol.OutcomeCompleted || script.calls.Load() != 3 {
		diagnostic, _ := json.Marshal(finished)
		t.Fatalf("preparation failure stopped continuation: calls=%d, run=%s, error=%v", script.calls.Load(), diagnostic, err)
	}
	before := script.observed.Load()
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
	if script.calls.Load() != 4 || !reflect.DeepEqual(before, script.recovered.Load()) {
		prior, _ := json.Marshal(before)
		after, _ := json.Marshal(script.recovered.Load())
		t.Fatalf("preparation results changed across restart: calls=%d, before=%s, after=%s", script.calls.Load(), prior, after)
	}
}

// preparationFailureScript proposes two shell calls the hook makes unexecutable,
// then records the tool results its third call observed and the fourth call,
// after restart, recovered.
type preparationFailureScript struct {
	calls               atomic.Int32
	observed, recovered atomic.Pointer[[]chat.ToolResult]
}

func (s *preparationFailureScript) Call(_ context.Context, request *chat.Request) (*chat.Response, error) {
	call := s.calls.Add(1)
	switch call {
	case 3:
		results := clonedToolResults(request)
		s.observed.Store(&results)
	case 4:
		results := clonedToolResults(request)
		s.recovered.Store(&results)
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
		Model: "claude-test", Usage: &chat.Usage{InputTokens: 2, OutputTokens: 1},
	})
}

// clonedToolResults copies every tool result the request carries, so later
// requests cannot alias what the test inspects.
func clonedToolResults(request *chat.Request) []chat.ToolResult {
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
	return results
}
