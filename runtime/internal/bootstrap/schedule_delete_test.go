package bootstrap

import (
	"context"
	json "encoding/json/v2"
	"fmt"
	"iter"
	"os"
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/internal/testsupport"

	framevalue "github.com/Tangerg/frame/value"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient"
)

func TestScheduleDeletionRefusesAnOutdatedReview(t *testing.T) {
	cfg := poMCPConfig(t)
	r := poMCPOpen(t, cfg)
	installed := poMCPActivate(r, filepath.Join(firstPartyPackages, "schedules"))
	r.poMCPTools(&protocol.MCPServerID{
		Origin: protocol.MCPOrigin{Type: protocol.MCPOriginInstallation, InstallationID: installed.ID},
		Name:   "schedules",
	}, scheduleToolsReady)
	saved := readScheduleCreate(r, scheduleCreateRequest(r, testsupport.SchedulesDraft{
		Title: "Reviewed schedule", Instructions: "Inspect reviewed work", Cron: "0 0 1 1 *", TimeZone: "UTC",
	}), "create-schedule")
	const updatedTitle = "Edited after delete confirmation opened"
	update := scheduleUpdateRequest(r, testsupport.SchedulesUpdateInput{
		Id: saved.Id, ExpectedRevision: saved.Revision, Title: new(updatedTitle),
	})
	if reply := r.must(delivery.PluginsCall, update, "edit-schedule").(*protocol.CallPluginResponse); reply.Failure != nil {
		t.Fatal(reply.Failure)
	}
	deletion := scheduleDeleteRequest(r, saved.Id, saved.Revision)
	refused := r.must(delivery.PluginsCall, deletion, "delete-reviewed-schedule").(*protocol.CallPluginResponse)
	if refused.Failure == nil || refused.Failure.Code != "revision" {
		t.Fatalf("stale confirmation deleted the newer schedule: %+v", refused)
	}
	page := readSchedulePage(r, protocol.PageQuery{})
	if len(page.Data) != 1 || page.Data[0].Title != updatedTitle {
		t.Fatalf("updated definition was lost: %+v", page)
	}
	current := page.Data[0]
	accepted := readScheduleDeletion(r, scheduleDeleteRequest(r, current.Id, current.Revision), "delete-current-schedule")
	if accepted.Id != current.Id || generatedInteger(t, accepted.Revision) != generatedInteger(t, current.Revision) {
		t.Fatalf("deletion acknowledgement = %+v, reviewed definition = %+v", accepted, current)
	}
	if page := readSchedulePage(r, protocol.PageQuery{}); len(page.Data) != 0 {
		t.Fatalf("confirmed deletion retained schedule: %+v", page)
	}
}

func TestScheduleDeletionReplaysItsExactReceiptAfterReplyLoss(t *testing.T) {
	source, evidence := managedMessageLossPackage(t, "schedules", testsupport.SchedulesDeleteV3Method, "response")
	cfg := poMCPConfig(t)
	r := poMCPOpen(t, cfg)
	installed := poMCPActivate(r, source)
	server := &protocol.MCPServerID{
		Origin: protocol.MCPOrigin{Type: protocol.MCPOriginInstallation, InstallationID: installed.ID},
		Name:   "schedules",
	}
	r.poMCPTools(server, scheduleToolsReady)
	saved := readScheduleCreate(r, scheduleCreateRequest(r, testsupport.SchedulesDraft{
		Instructions: "Retain my deletion receipt", Cron: "0 0 1 1 *", TimeZone: "UTC",
	}), "create-schedule")
	deletion := scheduleDeleteRequest(r, saved.Id, saved.Revision)
	const key = "lost-deletion"
	lost := r.endpoint.Invoke(t.Context(), delivery.PluginsCall, deletion, protocol.CallMetadata{
		IdempotencyKey: key, RequestMeta: protocol.RequestMeta{ProtocolVersion: new(protocol.Version)},
	})
	if lost.Failure == nil {
		t.Fatal("withheld deletion reply appeared successful")
	}
	if _, err := os.Stat(evidence); err != nil {
		t.Fatal(err)
	}
	r = reopenPluginRuntime(t, r, &cfg)
	r.poMCPTools(server, scheduleToolsReady)
	accepted := readScheduleDeletion(r, deletion, key)
	if accepted.Id != saved.Id || generatedInteger(t, accepted.Revision) != generatedInteger(t, saved.Revision) {
		t.Fatalf("recovered deletion = %+v, original = %+v", accepted, saved)
	}
	if page := readSchedulePage(r, protocol.PageQuery{}); len(page.Data) != 0 {
		t.Fatalf("replayed deletion recreated schedule: %+v", page)
	}
	fresh := r.must(delivery.PluginsCall, deletion, "another-deletion").(*protocol.CallPluginResponse)
	if fresh.Failure == nil || fresh.Failure.Code != "revision" {
		t.Fatalf("fresh deletion settled against a removed schedule: %+v", fresh)
	}
}

func TestScheduleModelDeletionReturnsTheInspectedRevision(t *testing.T) {
	const name = "schedules_delete_schedule"
	type deletionResult struct {
		ScheduleID string `json:"schedule_id"`
		Revision   uint64 `json:"revision"`
	}
	arguments := make(chan string, 1)
	observed := make(chan deletionResult, 1)
	model := delegateRestartModel{chat.ModelFunc(func(ctx context.Context, request *chat.Request) (*chat.Response, error) {
		for _, message := range request.Messages {
			for _, part := range message.Parts {
				if part.ToolResult == nil || part.ToolResult.Name != name {
					continue
				}
				if part.ToolResult.IsError {
					return nil, fmt.Errorf("schedule deletion failed: %v", part.ToolResult.Output)
				}
				var result deletionResult
				if err := json.Unmarshal(part.ToolResult.Output.Details, &result); err != nil {
					return nil, err
				}
				select {
				case observed <- result:
				default:
				}
				return completedTextResponse("deleted the inspected schedule"), nil
			}
		}
		call := chat.ToolCall{ID: "discover", Name: "search_tools", Arguments: `{"query":"select:` + name + `"}`}
		for _, tool := range request.Tools {
			if tool.Name == name {
				select {
				case input := <-arguments:
					call = chat.ToolCall{ID: "delete", Name: name, Arguments: input}
				case <-ctx.Done():
					return nil, context.Cause(ctx)
				}
				break
			}
		}
		message := chat.NewAssistantMessage(chat.NewToolCallPart(call))
		return chat.NewResponse(&chat.Output{Message: &message, FinishReason: chat.FinishReasonToolCalls}, nil)
	})}
	client, err := chatclient.New(model)
	if err != nil {
		t.Fatal(err)
	}
	cfg := poMCPConfig(t)
	cfg.modelProviders = testChatResolver(client)
	cfg.approvalMode = approval.ModeYolo
	r := poMCPOpen(t, cfg)
	installed := poMCPActivate(r, filepath.Join(firstPartyPackages, "schedules"))
	r.poMCPTools(&protocol.MCPServerID{
		Origin: protocol.MCPOrigin{Type: protocol.MCPOriginInstallation, InstallationID: installed.ID},
		Name:   "schedules",
	}, scheduleToolsReady)
	saved := readScheduleCreate(r, scheduleCreateRequest(r, testsupport.SchedulesDraft{
		Instructions: "Reviewed recurring work", Cron: "0 0 1 1 *", TimeZone: "UTC",
	}), "create-schedule")
	revision := generatedInteger(t, saved.Revision)
	arguments <- fmt.Sprintf(`{"schedule_id":%q,"expected_revision":%d}`, saved.Id, revision)
	close(arguments)
	session := r.must(delivery.SessionsCreate, protocol.CreateSessionRequest{}, "session").(*protocol.Session)
	started := r.endpoint.Invoke(t.Context(), delivery.RunsStart, protocol.StartRunRequest{
		SessionID: session.ID, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "Remove the inspected schedule."}},
	}, protocol.CallMetadata{IdempotencyKey: "delete-schedule-with-tool", RequestMeta: protocol.RequestMeta{ProtocolVersion: new(protocol.Version)}})
	if started.Failure != nil {
		t.Fatal(started.Failure)
	}
	events := iter.Seq2[protocol.RunEvent, error](func(yield func(protocol.RunEvent, error) bool) {
		for event, err := range started.Events {
			if !yield(event.(protocol.RunEvent), err) {
				return
			}
		}
	})
	waitForRunEvents(t, collectRunEvents(events), "schedule deletion tool")
	select {
	case result := <-observed:
		if result.ScheduleID != saved.Id || result.Revision != uint64(revision) {
			t.Fatalf("tool result = %+v, inspected schedule = %+v", result, saved)
		}
	default:
		t.Fatal("model did not receive the deletion receipt")
	}
	if page := readSchedulePage(r, protocol.PageQuery{}); len(page.Data) != 0 {
		t.Fatalf("model tool left confirmed deletion visible: %+v", page)
	}
}

func scheduleDeleteRequest(r *poMCPRuntime, id string, revision framevalue.Number) protocol.CallPluginRequest {
	r.t.Helper()
	request := scheduleQuery(r, protocol.PageQuery{})
	input, err := json.Marshal(testsupport.SchedulesDeleteInput{Id: id, ExpectedRevision: revision})
	if err != nil {
		r.t.Fatal(err)
	}
	request.Route.Method, request.Input = testsupport.SchedulesDeleteV3Method, string(input)
	return request
}

func readScheduleDeletion(r *poMCPRuntime, request protocol.CallPluginRequest, key string) testsupport.SchedulesDeleteResult {
	r.t.Helper()
	reply := r.must(delivery.PluginsCall, request, key).(*protocol.CallPluginResponse)
	if reply.Failure != nil {
		r.t.Fatal(reply.Failure)
	}
	var deleted testsupport.SchedulesDeleteResult
	if err := json.Unmarshal([]byte(reply.Output), &deleted, json.RejectUnknownMembers(true)); err != nil {
		r.t.Fatal(err)
	}
	return deleted
}
