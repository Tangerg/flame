package bootstrap

import (
	"context"
	"encoding/json"
	"reflect"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/protocol"
	"github.com/Tangerg/scope/core/chat"
)

func TestProtocolSettlesApprovalWhenEditedArgumentsCannotExecute(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FLAME_HOME", home)
	var calls atomic.Int32
	var recoveredToolResult atomic.Bool
	model := delegateRestartModel{chat.ModelFunc(func(_ context.Context, request *chat.Request) (*chat.Response, error) {
		call := calls.Add(1)
		if call == 3 {
			for _, message := range request.Messages {
				for _, part := range message.Parts {
					if part.ToolResult != nil && part.ToolResult.ID == "shell_approval" && part.ToolResult.IsError {
						recoveredToolResult.Store(true)
					}
				}
			}
		}
		message := chat.NewAssistantMessage(chat.NewTextPart("The edited tool could not execute."))
		finish := chat.FinishReasonStop
		if call == 1 {
			message = chat.NewAssistantMessage(chat.NewToolCallPart(chat.ToolCall{
				ID: "shell_approval", Name: "shell", Arguments: `{"command":"printf approved","description":"Print approved"}`,
			}))
			finish = chat.FinishReasonToolCalls
		}
		return chat.NewResponse(&chat.Output{Message: &message, FinishReason: finish}, &chat.ResponseMetadata{
			Model: "claude-test", Usage: chat.Usage{InputTokens: 2, OutputTokens: 1},
		})
	})}
	stores, err := persistence.Open(t.Context(), persistence.Config{
		DataDirectory: home, DefaultWorkspacePath: home,
	})
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
			ItemID: itemID,
			Response: protocol.InterruptResponseValue{
				Type: protocol.InterruptResponseApproval, Decision: protocol.ApprovalApprove,
				EditedArgs: map[string]any{"command": true, "description": "Invalid command type"},
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	waitForRunEvents(t, collectRunEvents(events), "invalid edited tool")
	snapshot, err := api.GetSessionSnapshot(ctx, protocol.GetSessionSnapshotRequest{SessionID: session.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Runs) != 1 || snapshot.Runs[0].Status != protocol.RunStatusFinished || calls.Load() != 2 {
		t.Fatalf("edited approval run did not settle: %+v, model calls=%d", snapshot.Runs, calls.Load())
	}
	var settled protocol.Item
	for _, item := range snapshot.Items {
		if item.ID != itemID {
			continue
		}
		if item.Status != protocol.ItemStatusIncomplete || item.ApprovalDecision != protocol.ApprovalApprove || item.Error == nil {
			diagnostic, _ := json.Marshal(item)
			t.Fatalf("accepted approval lost its failed Tool result: %s", diagnostic)
		}
		settled = item
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
}
