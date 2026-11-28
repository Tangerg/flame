package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	framecall "github.com/Tangerg/frame/call"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient"
)

type methodTestExecution struct {
	ID, State, Reason, Digest, Patch, Workspace, Base string
	Version                                           struct{ ID, Change string }
	Change, Review                                    *struct{ ID, SessionID string }
	Checks                                            []struct{ Slot, RunID, Status, Result string }
	Decision                                          *struct{ Digest, At string }
}

func TestMethodsRetainVersionAndOriginalSiblingAcrossRestart(t *testing.T) {
	source, git := taskWorktreeTestRepository(t)
	if err := os.WriteFile(filepath.Join(source, "message.txt"), []byte("original\n"), 0600); err != nil {
		t.Fatal(err)
	}
	git("add", "message.txt")
	git("commit", "--quiet", "-m", "initial")
	base := git("rev-parse", "HEAD")
	cfg := poMCPConfig(t)
	cfg.defaultWorkspacePath, cfg.approvalMode = source, approval.ModeBalanced
	var finishedChecks atomic.Int32
	model, err := chatclient.New(delegateRestartModel{chat.ModelFunc(methodRestartScript(t, &finishedChecks))})
	if err != nil {
		t.Fatal(err)
	}
	cfg.modelProviders = testChatResolver(model)
	r := poMCPOpen(t, cfg)
	project := r.must(delivery.Name("projects.create"), protocol.CreateProjectRequest{Name: "Versioned changes"}, "create-method-project").(*protocol.Project)
	tree := r.must(delivery.TaskWorktreesCreate, protocol.CreateTaskWorktreeRequest{
		Source: protocol.WorkspaceRef{Path: source}, Base: base,
	}, "create-method-worktree").(*protocol.TaskWorktree)
	installed := poMCPActivate(r, filepath.Join(firstPartyPackages, "methods"))
	waitConnected := func() {
		t.Helper()
		waitCallContributorState(t, r, installed.ID, protocol.MCPServerConnected)
	}
	waitConnected()
	contract, err := plugincontract.ParseCapability([]byte(installed.Selected.Contributions[0].Contract))
	if err != nil {
		t.Fatal(err)
	}
	invoke := func(method, key string, args map[string]string) *protocol.CallPluginResponse {
		t.Helper()
		args["projectId"] = project.ID
		input, err := json.Marshal(args)
		if err != nil {
			t.Fatal(err)
		}
		return r.must(delivery.PluginsCall, protocol.CallPluginRequest{
			Route: protocol.PluginCallRoute{
				InstallationID: installed.ID, Digest: installed.Selected.Digest, Contribution: "project-methods",
				Contract: contract.Identity().String(), Method: "flame.methods/" + method + ".v1",
				Target: framecall.Target{Scope: extension.ScopeInstallation, Reference: installed.ID},
			}, Input: string(input),
		}, key).(*protocol.CallPluginResponse)
	}
	decode := func(reply *protocol.CallPluginResponse, target any) {
		t.Helper()
		if reply.Failure != nil {
			t.Fatalf("methods operation failed: %+v", reply.Failure)
		}
		if err := json.Unmarshal([]byte(reply.Output), target); err != nil {
			t.Fatal(err)
		}
	}
	save := func(key, change string) string {
		t.Helper()
		var version struct{ ID string }
		decode(invoke("save", key, map[string]string{
			"title": "Update the message", "change": change, "reviewA": "Inspect original change A.", "reviewB": "Inspect original change B.",
			"provider": "anthropic", "model": "claude-test",
		}), &version)
		return version.ID
	}
	version := save("method-first-version", "Implement the original change.")
	args := map[string]string{"versionId": version, "worktreeId": tree.ID}
	var started methodTestExecution
	decode(invoke("start", "execute-original-method", args), &started)
	if started.ID == "" || started.Version.ID != version || started.Workspace != tree.Workspace.Path || started.Base != base {
		t.Fatalf("admission lost the original worktree or version: %+v", started)
	}
	if latest := save("method-future-version", "Use this change in the next execution."); latest == version {
		t.Fatal("changing a method reused the active version")
	}
	if duplicate := invoke("start", "parallel-worktree-conflict", args); duplicate.Failure == nil {
		t.Fatal("a second method was admitted on the original worktree")
	}
	read := func() methodTestExecution {
		t.Helper()
		var observed methodTestExecution
		decode(invoke("execution", "", map[string]string{"id": started.ID}), &observed)
		return observed
	}
	wait := func(until func(methodTestExecution) bool) methodTestExecution {
		t.Helper()
		ctx, cancel := context.WithTimeout(t.Context(), 35*time.Second)
		defer cancel()
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		for {
			current := read()
			if current.State == "blocked" {
				t.Fatalf("method blocked: %+v", current)
			}
			if until(current) {
				return current
			}
			select {
			case <-ticker.C:
			case <-ctx.Done():
				t.Fatalf("method did not progress from %+v", current)
			}
		}
	}
	meta := protocol.RequestMeta{ProtocolVersion: new(protocol.Version), ClientCapabilities: &protocol.ClientCapabilities{
		InterruptTypes: []protocol.InterruptType{protocol.InterruptQuestion, protocol.InterruptApproval},
		Features:       map[string]protocol.FeaturePreference{protocol.FeatureChildRuns: {Enabled: true}},
	}}
	ctx := delivery.WithRequestMeta(t.Context(), meta)
	var pendingID string
	paused := wait(func(current methodTestExecution) bool {
		if current.Review == nil || len(current.Checks) != 2 || current.Checks[0].Status != "passed" {
			return false
		}
		pending, err := r.api.ListInterrupts(ctx, protocol.ListInterruptsRequest{RootRunID: current.Review.ID})
		if err != nil {
			t.Fatal(err)
		}
		if len(pending.Data) == 1 && len(pending.Data[0].Interrupts) == 1 {
			pendingID = pending.Data[0].Interrupts[0].ItemID
		}
		return pendingID != ""
	})
	if paused.Version.ID != version || !strings.Contains(paused.Patch, "+updated") || paused.Checks[0].Result != "PASS: observed the original change" {
		t.Fatalf("review inputs or retained evidence changed: %+v", paused)
	}
	r = reopenPluginRuntime(t, r, &cfg)
	waitConnected()
	afterRestart := read()
	if !slices.Equal(afterRestart.Checks, paused.Checks) || *afterRestart.Review != *paused.Review {
		t.Fatalf("restart replaced the review tree: %+v", afterRestart)
	}
	_, events, err := r.api.ResumeRun(ctx, protocol.ResumeRunRequest{
		RunID: paused.Review.ID,
		Responses: []protocol.InterruptResponse{{
			ItemID: pendingID, Response: protocol.InterruptResponseValue{Type: protocol.InterruptResponseAnswer, Answers: [][]string{{"Proceed"}}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	waitForRunEvents(t, collectRunEvents(events), "original method review")
	report := wait(func(current methodTestExecution) bool { return current.State == "awaiting_approval" })
	if finishedChecks.Load() != 1 || len(report.Checks) != 2 || report.Checks[0] != paused.Checks[0] ||
		report.Checks[1].RunID != paused.Checks[1].RunID || report.Digest == "" || report.Version.ID != version {
		t.Fatalf("resumed report replaced an original phase: %+v (A completed %d times)", report, finishedChecks.Load())
	}
	r = reopenPluginRuntime(t, r, &cfg)
	waitConnected()
	if retained := read(); retained.Digest != report.Digest || retained.State != report.State || retained.Decision != nil {
		t.Fatalf("restart lost the pending human decision: %+v", retained)
	}
	changedFile := filepath.Join(tree.Workspace.Path, "message.txt")
	if err := os.WriteFile(changedFile, []byte("changed after review\n"), 0600); err != nil {
		t.Fatal(err)
	}
	decision := map[string]string{"id": started.ID, "digest": report.Digest}
	if refused := invoke("approve", "stale-method-approval", decision); refused.Failure == nil {
		t.Fatal("the original report approved an alternate patch")
	}
	if err := os.WriteFile(changedFile, []byte("updated\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var approved, replay methodTestExecution
	decode(invoke("approve", "approve-original-method", decision), &approved)
	decode(invoke("approve", "approve-original-method", decision), &replay)
	if approved.State != "approved" || approved.Decision == nil || replay.Decision == nil || approved.Decision.Digest != report.Digest ||
		*approved.Decision != *replay.Decision || *approved.Decision != *read().Decision {
		t.Fatalf("original human decision failed to settle once: %+v %+v", approved, replay)
	}
	if origin, err := os.ReadFile(filepath.Join(source, "message.txt")); err != nil || string(origin) != "original\n" {
		t.Fatalf("method wrote outside its managed worktree: %q, %v", origin, err)
	}
}

func methodRestartScript(t *testing.T, finishedChecks *atomic.Int32) chat.ModelFunc {
	return func(_ context.Context, request *chat.Request) (*chat.Response, error) {
		var prompt string
		for _, message := range request.Messages {
			if message.Role == chat.RoleUser {
				prompt = message.Text()
				break
			}
		}
		if len(request.Tools) == 0 {
			return completedTextResponse("Versioned change"), nil
		}
		if strings.HasPrefix(prompt, "Make the requested changes") {
			if !strings.Contains(prompt, "Implement the original change.") {
				t.Error("the change Run received instructions for another version")
			}
			if result := toolResultNamed(request, "edit"); result != nil {
				if result.IsError {
					return nil, fmt.Errorf("method edit failed: %+v", result)
				}
				return completedTextResponse("Applied the original change"), nil
			}
			if result := toolResultNamed(request, "read"); result != nil {
				return filewatchToolCall(chat.ToolCall{ID: "method-edit", Name: "edit", Arguments: `{"path":"message.txt","old_string":"original","new_string":"updated"}`})
			}
			return filewatchToolCall(chat.ToolCall{ID: "method-read", Name: "read", Arguments: `{"path":"message.txt"}`})
		}
		var tools []string
		for _, tool := range request.Tools {
			tools = append(tools, tool.Name)
		}
		slices.Sort(tools)
		want := []string{"ask_user", "glob", "grep", "read"}
		if strings.HasPrefix(prompt, "Delegate both checks") {
			want = []string{"ask_user", "delegate_task", "glob", "grep", "read"}
			if !slices.Equal(tools, want) {
				t.Errorf("original review parent offered %v, want %v", tools, want)
			}
			if toolResultNamed(request, "delegate_task") != nil {
				return completedTextResponse("Both original checks finished."), nil
			}
			_, payload, _ := strings.Cut(prompt, "\n\n")
			var assignments [2]json.RawMessage
			if err := json.Unmarshal([]byte(payload), &assignments); err != nil {
				return nil, fmt.Errorf("decode original review assignments: %w", err)
			}
			message := chat.NewAssistantMessage(
				chat.NewToolCallPart(chat.ToolCall{ID: "method-review-a", Name: "delegate_task", Arguments: string(assignments[0])}),
				chat.NewToolCallPart(chat.ToolCall{ID: "method-review-b", Name: "delegate_task", Arguments: string(assignments[1])}),
			)
			return chat.NewResponse(&chat.Output{Message: &message, FinishReason: chat.FinishReasonToolCalls},
				&chat.ResponseMetadata{Model: "claude-test", Usage: &chat.Usage{InputTokens: 2, OutputTokens: 1}})
		}
		if !slices.Equal(tools, want) {
			t.Errorf("original review child offered %v, want %v", tools, want)
		}
		switch {
		case strings.Contains(prompt, "Inspect original change A."):
			finishedChecks.Add(1)
			return completedTextResponse("PASS: observed the original change"), nil
		case strings.Contains(prompt, "Inspect original change B."):
			if toolResultNamed(request, "ask_user") != nil {
				return completedTextResponse("PASS: received the original continuation"), nil
			}
			return filewatchToolCall(chat.ToolCall{ID: "method-review-question", Name: "ask_user", Arguments: `{"questions":[{"question":"Continue review B?"}]}`})
		default:
			return nil, fmt.Errorf("unrecognized method instruction %q", prompt)
		}
	}
}
