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

const (
	scheduledRunTitle   = "Scheduled Workspace Check"
	scheduledTitleInput = 17
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
	script := &manualRunScript{t: t}
	script.receipt.Store("")
	model := delegateRestartModel{chat.ModelFunc(script.Call)}
	client, err := chatclient.New(model)
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
			script.modelName = tool.ModelName
		}
	}
	if script.modelName == "" {
		t.Fatal("manual schedule tool was not published")
	}
	definition := readScheduleCreate(r, scheduleCreateRequest(r, testsupport.SchedulesDraft{Instructions: "Run the captured fixture once", Cron: "0 * * * *", TimeZone: "UTC", Enabled: false}), "manual-definition")
	script.scheduleID = definition.Id
	session := r.must(delivery.SessionsCreate, protocol.CreateSessionRequest{Title: "Manual schedule caller"}, "manual-session").(*protocol.Session)
	started := r.endpoint.Invoke(t.Context(), delivery.RunsStart, protocol.StartRunRequest{SessionID: session.ID, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "Run this schedule once now."}}}, protocol.CallMetadata{IdempotencyKey: "manual-from-model", RequestMeta: protocol.RequestMeta{ProtocolVersion: new(protocol.Version)}})
	if started.Failure != nil {
		t.Fatal(started.Failure)
	}
	for _, err := range started.Events {
		if err != nil {
			t.Fatal(err)
		}
	}
	var accepted testsupport.SchedulesRunReference
	if lost {
		select {
		case committed := <-opening:
			child := r.must(delivery.RunsGet, protocol.GetRunRequest{RunID: committed.RunID}, "").(*protocol.RunRef)
			accepted.RunId, accepted.SessionId = child.ID, child.SessionID
		default:
			t.Fatal("unknown Tool did not retain the committed Runtime opening")
		}
		r.must(delivery.SessionsDelete, protocol.DeleteSessionRequest{SessionID: session.ID}, "delete-original-tool-session")
		r = reopenPluginRuntime(t, r, &cfg)
		r.poMCPTools(server, scheduleToolsReady)
		deadline := time.Now().Add(5 * time.Second)
		for {
			page := readSchedulePage(r, protocol.PageQuery{})
			if len(page.Data) == 1 && page.Data[0].LastRunAt != nil {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("original model manual capture did not reconcile after restart: %+v", page)
			}
			time.Sleep(10 * time.Millisecond)
		}
	} else {
		if err := json.Unmarshal([]byte(script.receipt.Load().(string)), &accepted, json.RejectUnknownMembers(true)); err != nil || accepted.RunId == "" || accepted.SessionId == "" {
			root := r.must(delivery.RunsGet, protocol.GetRunRequest{RunID: started.Value.(*protocol.StartRunResponse).RunID}, "").(*protocol.RunRef)
			encoded, _ := json.Marshal(root)
			t.Fatalf("model did not receive original Run acceptance: %+v, %v; issued=%t exchanges=%d run=%s", accepted, err, script.issued.Load(), script.exchanges.Load(), encoded)
		}
	}
	child := r.must(delivery.RunsGet, protocol.GetRunRequest{RunID: accepted.RunId}, "").(*protocol.RunRef)
	if child.SessionID != accepted.SessionId || child.SessionID == session.ID {
		t.Fatalf("manual tool did not enter the existing new-Session owner: %+v", child)
	}
	createdSession := r.must(delivery.SessionsGet, protocol.GetSessionRequest{SessionID: accepted.SessionId}, "").(*protocol.Session)
	if createdSession.Workspace.Ref.Path != session.Workspace.Ref.Path {
		t.Fatalf("omitted schedule workspace did not use Runtime's default: %+v", createdSession)
	}
	if !lost {
		verifyScheduledRunTitle(r, createdSession, session.ID)
	}
	page := readSchedulePage(r, protocol.PageQuery{})
	if len(page.Data) != 1 || page.Data[0].Id != definition.Id || page.Data[0].LastRunAt == nil {
		t.Fatalf("manual acceptance failed to settle its private definition: %+v", page)
	}
}

func verifyScheduledRunTitle(r *poMCPRuntime, scheduled *protocol.Session, initiatingSessionID string) {
	t := r.t
	t.Helper()
	ctx, stop := context.WithTimeout(t.Context(), lifecycleWaitBudget(t))
	defer stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for scheduled.Title == "" {
		select {
		case <-ctx.Done():
			t.Fatalf("scheduled Session never received a title: %v", ctx.Err())
		case <-ticker.C:
		}
		scheduled = r.must(delivery.SessionsGet, protocol.GetSessionRequest{SessionID: scheduled.ID}, "").(*protocol.Session)
	}
	if scheduled.Title != scheduledRunTitle {
		t.Fatalf("scheduled Session title = %q, want generated title %q", scheduled.Title, scheduledRunTitle)
	}
	for _, usage := range []struct {
		sessionID string
		wantInput int64
	}{
		{scheduled.ID, scheduledTitleInput},
		{initiatingSessionID, 0},
	} {
		got := r.must(delivery.UsageSession, protocol.SessionUsageRequest{SessionID: usage.sessionID}, "").(*protocol.SessionUsage)
		if got.Total.InputTokens != usage.wantInput {
			t.Errorf("Session %q input tokens = %d, want %d", usage.sessionID, got.Total.InputTokens, usage.wantInput)
		}
	}
}

// manualRunScript discovers the installed run_schedule tool, calls it once for
// the captured schedule and keeps the model-visible receipt it returned.
type manualRunScript struct {
	t                     *testing.T
	modelName, scheduleID string
	issued                atomic.Bool
	exchanges             atomic.Int32
	receipt               atomic.Value
}

func (s *manualRunScript) Call(_ context.Context, request *chat.Request) (*chat.Response, error) {
	s.exchanges.Add(1)
	if result := toolResultNamed(request, s.modelName); result != nil {
		if result.IsError {
			s.t.Errorf("installed manual Run was refused: %+v", result.Output)
		}
		text, ok := result.Output.Text()
		if !ok {
			s.t.Error("manual Run result lost its model-visible JSON output")
		}
		s.receipt.Store(text)
		return completedTextResponse("manual work accepted"), nil
	}
	if len(request.Tools) == 0 {
		response := completedTextResponse(scheduledRunTitle)
		response.Metadata = &chat.ResponseMetadata{Usage: &chat.Usage{InputTokens: scheduledTitleInput}}
		return response, nil
	}
	if s.issued.Load() {
		return completedTextResponse("scheduled work finished"), nil
	}
	call := deferredToolCall(request, "discover-manual", chat.ToolCall{ID: "manual-schedule-call", Name: s.modelName, Arguments: `{"id":"` + s.scheduleID + `"}`})
	if call.Name == s.modelName {
		s.issued.Store(true)
	}
	message := chat.NewAssistantMessage(chat.NewToolCallPart(call))
	return chat.NewResponse(&chat.Output{Message: &message, FinishReason: chat.FinishReasonToolCalls}, nil)
}
