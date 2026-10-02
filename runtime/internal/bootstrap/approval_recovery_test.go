package bootstrap

import (
	"context"
	json "encoding/json/v2"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/protocol"
	"github.com/Tangerg/scope/core/chat"
)

func TestProtocolPreservesUnsuccessfulApprovalResultsAcrossRestart(t *testing.T) {
	for _, test := range []struct {
		name     string
		response protocol.InterruptResponseValue
		reason   string
	}{
		{name: "invalid approved arguments", response: protocol.InterruptResponseValue{
			Type: protocol.InterruptResponseApproval, Decision: protocol.ApprovalApprove,
			EditedArgs: map[string]any{"command": true, "description": "Invalid command type"},
		}},
		{name: "invalid remembered arguments", response: protocol.InterruptResponseValue{
			Type: protocol.InterruptResponseApproval, Decision: protocol.ApprovalApprove,
			EditedArgs: map[string]any{"command": true, "description": "Invalid command type"},
			Remember:   &protocol.RememberScope{Scope: protocol.RememberGlobal},
		}},
		{name: "invalid remembered schema", response: protocol.InterruptResponseValue{
			Type: protocol.InterruptResponseApproval, Decision: protocol.ApprovalApprove,
			EditedArgs: map[string]any{"command": "printf approved", "description": true},
			Remember:   &protocol.RememberScope{Scope: protocol.RememberGlobal},
		}},
		{name: "user refusal", response: protocol.InterruptResponseValue{
			Type: protocol.InterruptResponseApproval, Decision: protocol.ApprovalDeny,
			Reason: "inspect the existing file before attempting a write",
		}, reason: "inspect the existing file before attempting a write"},
	} {
		t.Run(test.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("FLAME_HOME", home)
			var calls atomic.Int32
			var recoveredToolResult atomic.Bool
			model := delegateRestartModel{chat.ModelFunc(func(_ context.Context, request *chat.Request) (*chat.Response, error) {
				call := calls.Add(1)
				if call >= 2 {
					matches := 0
					for _, message := range request.Messages {
						for _, part := range message.Parts {
							if part.ToolResult != nil && part.ToolResult.ID == "shell_approval" && part.ToolResult.IsError {
								matches++
								if test.reason != "" && !reflect.DeepEqual(part.ToolResult.Output, chat.NewTextToolOutput(test.reason)) {
									return nil, fmt.Errorf("approval refusal lost its reason: %+v", part.ToolResult.Output)
								}
							}
						}
					}
					if matches != 1 {
						return nil, fmt.Errorf("model call %d has %d approval results, want one", call, matches)
					}
					recoveredToolResult.Store(call == 3)
				}
				message := chat.NewAssistantMessage(chat.NewTextPart("The edited tool could not execute."))
				finish := chat.FinishReasonStop
				if call == 1 {
					message = chat.NewAssistantMessage(chat.NewToolCallPart(chat.ToolCall{
						ID: "shell_approval", Name: "shell", Arguments: `{"command":"printf approved > approval-side-effect.txt","description":"Write approved"}`,
					}))
					finish = chat.FinishReasonToolCalls
				}
				return chat.NewResponse(&chat.Output{Message: &message, FinishReason: finish}, &chat.ResponseMetadata{
					Model: "claude-test", Usage: &chat.Usage{InputTokens: 2, OutputTokens: 1},
				})
			})}
			stores, err := persistence.Open(t.Context(), persistence.Config{DataDirectory: home})
			if err != nil {
				t.Fatal(err)
			}
			cfg := protocolRuntimeConfig(t, stores, model)
			cfg.ApprovalMode = approval.ModeSafe
			host, api := buildProtocolRuntime(t, cfg, home)
			t.Cleanup(func() {
				if err := host.Close(); err != nil {
					t.Error(err)
				}
			})
			ctx := delivery.WithRequestMeta(t.Context(), protocol.RequestMeta{
				ProtocolVersion:    protocol.ProtocolVersion,
				ClientCapabilities: &protocol.ClientCapabilities{InterruptTypes: []protocol.InterruptType{protocol.InterruptApproval}},
			})
			session, err := api.CreateSession(ctx, protocol.CreateSessionRequest{
				Workspace: &protocol.WorkspaceRef{Path: home}, Title: "edited approval",
			})
			if err != nil {
				t.Fatal(err)
			}
			started, events, err := api.StartRun(ctx, protocol.StartRunRequest{
				SessionID: session.ID, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "Run the tool after approval."}},
			})
			if err != nil {
				t.Fatal(err)
			}
			waitForRunEvents(t, collectRunEvents(events), "approval boundary")
			pending, err := api.ListInterrupts(ctx, protocol.ListInterruptsRequest{RootRunID: started.RunID})
			if err != nil || len(pending.Data) != 1 || len(pending.Data[0].Interrupts) != 1 {
				t.Fatalf("approval boundary = %+v, %v", pending, err)
			}
			itemID := pending.Data[0].Interrupts[0].ItemID
			_, events, err = api.ResumeRun(ctx, protocol.ResumeRunRequest{
				RunID: started.RunID,
				Responses: []protocol.InterruptResponse{{
					ItemID:   itemID,
					Response: test.response,
				}},
			})
			if err != nil {
				t.Fatal(err)
			}
			waitForRunEvents(t, collectRunEvents(events), "invalid edited tool")
			rules, err := api.ListApprovalRules(ctx, protocol.ListApprovalRulesRequest{SessionID: session.ID})
			if err != nil || len(rules.Rules) != 0 {
				t.Fatalf("unsuccessful approval saved a standing rule: %+v, %v", rules, err)
			}
			snapshot, err := api.GetSessionSnapshot(ctx, protocol.GetSessionSnapshotRequest{SessionID: session.ID})
			if err != nil {
				t.Fatal(err)
			}
			if len(snapshot.Runs) != 1 || snapshot.Runs[0].Status != protocol.RunStatusFinished || snapshot.Runs[0].Outcome == nil || snapshot.Runs[0].Outcome.Type != protocol.OutcomeCompleted || calls.Load() != 2 {
				t.Fatalf("edited approval run did not settle: %+v, model calls=%d", snapshot.Runs, calls.Load())
			}
			var settled protocol.Item
			for _, item := range snapshot.Items {
				if item.ID != itemID {
					continue
				}
				if item.Status != protocol.ItemStatusIncomplete || item.ApprovalDecision != test.response.Decision || item.Error == nil {
					diagnostic, _ := json.Marshal(item)
					t.Fatalf("accepted approval lost its failed Tool result: %s", diagnostic)
				}
				settled = item
			}
			if _, err := os.Stat(filepath.Join(home, "approval-side-effect.txt")); !os.IsNotExist(err) {
				t.Fatalf("refused or invalid approval executed the Tool: %v", err)
			}
			if settled.ID == "" {
				t.Fatal("accepted approval Item disappeared")
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
				t.Fatalf("settled approval changed across restart: %+v, %v", afterRestart, err)
			}
			_, events, err = api.StartRun(ctx, protocol.StartRunRequest{
				SessionID: session.ID, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "Continue after the invalid edit."}},
			})
			if err != nil {
				t.Fatal(err)
			}
			waitForRunEvents(t, collectRunEvents(events), "next run after invalid edit")
			if calls.Load() != 3 || !recoveredToolResult.Load() {
				t.Fatalf("next Run lost the settled Tool result: model calls=%d, result=%t", calls.Load(), recoveredToolResult.Load())
			}
		})
	}
}

func TestProtocolRemembersEditedApprovalAfterWaitingRestart(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FLAME_HOME", home)
	const original = "printf original > original.txt"
	const approved = "printf approved > approved.txt # literal [*?"
	var calls atomic.Int32
	model := delegateRestartModel{chat.ModelFunc(func(_ context.Context, _ *chat.Request) (*chat.Response, error) {
		message := chat.NewAssistantMessage(chat.NewTextPart("Approved command completed."))
		finish := chat.FinishReasonStop
		if calls.Add(1) == 1 {
			arguments, err := json.Marshal(map[string]string{"command": original, "description": "Write original"})
			if err != nil {
				return nil, err
			}
			message = chat.NewAssistantMessage(chat.NewToolCallPart(chat.ToolCall{
				ID: "shell_approval", Name: "shell", Arguments: string(arguments),
			}))
			finish = chat.FinishReasonToolCalls
		}
		return chat.NewResponse(&chat.Output{Message: &message, FinishReason: finish}, &chat.ResponseMetadata{
			Model: "claude-test", Usage: &chat.Usage{InputTokens: 2, OutputTokens: 1},
		})
	})}
	ctx := delivery.WithRequestMeta(t.Context(), protocol.RequestMeta{
		ProtocolVersion:    protocol.ProtocolVersion,
		ClientCapabilities: &protocol.ClientCapabilities{InterruptTypes: []protocol.InterruptType{protocol.InterruptApproval}},
	})
	first, api := openProtocolRuntime(t, model)
	t.Cleanup(func() {
		if err := first.Close(); err != nil {
			t.Error(err)
		}
	})
	session, err := api.CreateSession(ctx, protocol.CreateSessionRequest{
		Workspace: &protocol.WorkspaceRef{Path: home}, Title: "remember edited approval",
	})
	if err != nil {
		t.Fatal(err)
	}
	started, events, err := api.StartRun(ctx, protocol.StartRunRequest{
		SessionID: session.ID, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "Run the tool after approval."}},
	})
	if err != nil {
		t.Fatal(err)
	}
	waitForRunEvents(t, collectRunEvents(events), "approval before restart")
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	restarted, api := openProtocolRuntime(t, model)
	t.Cleanup(func() {
		if err := restarted.Close(); err != nil {
			t.Error(err)
		}
	})
	pending, err := api.ListInterrupts(ctx, protocol.ListInterruptsRequest{RootRunID: started.RunID})
	if err != nil || len(pending.Data) != 1 || len(pending.Data[0].Interrupts) != 1 {
		t.Fatalf("restored approval = %+v, %v", pending, err)
	}
	_, events, err = api.ResumeRun(ctx, protocol.ResumeRunRequest{
		RunID: started.RunID,
		Responses: []protocol.InterruptResponse{{
			ItemID: pending.Data[0].Interrupts[0].ItemID,
			Response: protocol.InterruptResponseValue{
				Type: protocol.InterruptResponseApproval, Decision: protocol.ApprovalApprove,
				EditedArgs: map[string]any{"command": approved, "description": "Write approved"},
				Remember:   &protocol.RememberScope{Scope: protocol.RememberGlobal},
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	waitForRunEvents(t, collectRunEvents(events), "edited approval after restart")
	if content, err := os.ReadFile(filepath.Join(home, "approved.txt")); err != nil || string(content) != "approved" {
		t.Fatalf("confirmed command did not execute: %q, %v", content, err)
	}
	if _, err := os.Stat(filepath.Join(home, "original.txt")); !os.IsNotExist(err) {
		t.Fatalf("original command executed: %v", err)
	}
	if calls.Load() != 2 {
		t.Fatalf("model calls = %d, want 2", calls.Load())
	}
	if err := restarted.Close(); err != nil {
		t.Fatal(err)
	}
	cold, api := openProtocolRuntime(t, model)
	t.Cleanup(func() {
		if err := cold.Close(); err != nil {
			t.Error(err)
		}
	})
	rules, err := api.ListApprovalRules(ctx, protocol.ListApprovalRulesRequest{SessionID: session.ID})
	if err != nil || len(rules.Rules) != 1 {
		t.Fatalf("durable rules = %+v, %v", rules, err)
	}
	rule := rules.Rules[0]
	if rule.Tool != (protocol.ToolRef{Type: protocol.ToolRefBuiltIn, Name: "shell"}) ||
		rule.Subject != (protocol.ApprovalSubject{Type: protocol.ApprovalSubjectExact, Value: approved}) || rule.Decision != protocol.ApprovalRuleDecisionAllow ||
		rule.Scope != protocol.ApprovalRuleScopeGlobal || rule.Stale {
		t.Fatalf("durable rule differs from confirmed invocation: %+v", rule)
	}
}
