package bootstrap

import (
	"context"
	json "encoding/json/v2"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient"
)

func TestScheduleModelManualRunUsesOriginalInstalledCallerAndRunAdmission(t *testing.T) {
	for _, lost := range []bool{false, true} {
		name := "accepted"
		if lost {
			name = "lost opening acknowledgement"
		}
		t.Run(name, func(t *testing.T) { scheduleModelManualRun(t, lost) })
	}
}

func scheduleModelManualRun(t *testing.T, lost bool) {
	t.Helper()
	var issued atomic.Bool
	var exchanges atomic.Int32
	var receipt atomic.Value
	receipt.Store("")
	var modelName, scheduleID string
	model := delegateRestartModel{chat.ModelFunc(func(_ context.Context, request *chat.Request) (*chat.Response, error) {
		exchanges.Add(1)
		for _, message := range request.Messages {
			for _, part := range message.Parts {
				if part.ToolResult != nil && part.ToolResult.Name == modelName {
					if part.ToolResult.IsError {
						t.Errorf("installed manual Run was refused: %+v", part.ToolResult.Output)
					}
					text, ok := part.ToolResult.Output.Text()
					if !ok {
						t.Error("manual Run result lost its model-visible JSON output")
					}
					receipt.Store(text)
					return completedTextResponse("manual work accepted"), nil
				}
			}
		}
		if issued.Load() || len(request.Tools) == 0 {
			return completedTextResponse("scheduled work finished"), nil
		}
		call := chat.ToolCall{ID: "discover-manual", Name: "search_tools", Arguments: `{"query":"select:` + modelName + `"}`}
		for _, definition := range request.Tools {
			if definition.Name == modelName {
				issued.Store(true)
				call = chat.ToolCall{ID: "manual-schedule-call", Name: modelName, Arguments: `{"ID":"` + scheduleID + `"}`}
			}
		}
		message := chat.NewAssistantMessage(chat.NewToolCallPart(call))
		return chat.NewResponse(&chat.Output{Message: &message, FinishReason: chat.FinishReasonToolCalls}, nil)
	})}
	client, err := chatclient.New(model, chatclient.Config{})
	if err != nil {
		t.Fatal(err)
	}
	cfg := poMCPConfig(t)
	cfg.modelProviders, cfg.approvalMode = testChatResolver(client), approval.ModeYolo
	var r *poMCPRuntime
	var opening <-chan protocol.StartRunResponse
	if lost {
		r, opening = schedulesWithLostOpening(t, cfg)
	} else {
		r = poMCPOpen(t, cfg)
	}
	installed := poMCPActivate(r, filepath.Join(firstPartyPackages, "schedules"))
	server := &protocol.MCPServerID{Origin: protocol.MCPOrigin{Type: protocol.MCPOriginInstallation, InstallationID: installed.ID}, Name: "schedules"}
	tools := r.poMCPTools(server, scheduleToolsReady)
	for _, tool := range tools {
		if tool.Name == "run_schedule" {
			modelName = tool.ModelName
		}
	}
	if modelName == "" {
		t.Fatal("manual schedule tool was not published")
	}
	definition := readScheduleCreate(r, scheduleCreateRequest(r, testsupport.SchedulesCreateV1Input{Instructions: "Run the captured fixture once", Cron: "@hourly", Enabled: false}), "manual-definition")
	scheduleID = definition.ID
	session := r.must(delivery.SessionsCreate, protocol.CreateSessionRequest{Title: "Manual schedule caller"}, "manual-session").(*protocol.Session)
	started := r.endpoint.Invoke(t.Context(), delivery.RunsStart, protocol.StartRunRequest{SessionID: session.ID, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "Run this schedule once now."}}}, delivery.Options{IdempotencyKey: "manual-from-model", RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.Version}})
	if started.Failure != nil {
		t.Fatal(started.Failure)
	}
	for _, err := range started.Events {
		if err != nil {
			t.Fatal(err)
		}
	}
	var accepted testsupport.SchedulesRunNowV1Output
	if lost {
		select {
		case committed := <-opening:
			child := r.must(delivery.RunsGet, protocol.GetRunRequest{RunID: committed.RunID}, "").(*protocol.RunRef)
			accepted.RunID, accepted.SessionID = child.ID, child.SessionID
		default:
			t.Fatal("unknown Tool did not retain the committed Runtime opening")
		}
		r.must(delivery.SessionsDelete, protocol.DeleteSessionRequest{SessionID: session.ID}, "delete-original-tool-session")
		r = reopenPluginRuntime(t, r, &cfg)
		r.poMCPTools(server, scheduleToolsReady)
		deadline := time.Now().Add(5 * time.Second)
		for {
			page := readSchedulePage(r, protocol.PageQuery{})
			if len(page.Data) == 1 && !page.Data[0].LastRunAt.IsZero() {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("original model manual capture did not reconcile after restart: %+v", page)
			}
			time.Sleep(10 * time.Millisecond)
		}
	} else {
		if err := json.Unmarshal([]byte(receipt.Load().(string)), &accepted, json.RejectUnknownMembers(true)); err != nil || accepted.RunID == "" || accepted.SessionID == "" {
			root := r.must(delivery.RunsGet, protocol.GetRunRequest{RunID: started.Value.(*protocol.StartRunResponse).RunID}, "").(*protocol.RunRef)
			encoded, _ := json.Marshal(root)
			t.Fatalf("model did not receive original Run acceptance: %+v, %v; issued=%t exchanges=%d run=%s", accepted, err, issued.Load(), exchanges.Load(), encoded)
		}
	}
	child := r.must(delivery.RunsGet, protocol.GetRunRequest{RunID: accepted.RunID}, "").(*protocol.RunRef)
	if child.SessionID != accepted.SessionID || child.SessionID == session.ID {
		t.Fatalf("manual tool did not enter the existing new-Session owner: %+v", child)
	}
	createdSession := r.must(delivery.SessionsGet, protocol.GetSessionRequest{SessionID: accepted.SessionID}, "").(*protocol.Session)
	if createdSession.Workspace.Ref.Path != session.Workspace.Ref.Path {
		t.Fatalf("omitted schedule workspace did not use Runtime's default: %+v", createdSession)
	}
	page := readSchedulePage(r, protocol.PageQuery{})
	if len(page.Data) != 1 || page.Data[0].ID != definition.ID || page.Data[0].LastRunAt.IsZero() {
		t.Fatalf("manual acceptance failed to settle its private definition: %+v", page)
	}
}
