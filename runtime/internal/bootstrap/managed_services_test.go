package bootstrap

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"iter"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/protocol"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient"
)

func TestScheduleModelToolCommitsToBackendAndInvalidatesHumanReads(t *testing.T) {
	const name = "schedules_create_schedule"
	model := delegateRestartModel{chat.ModelFunc(func(_ context.Context, request *chat.Request) (*chat.Response, error) {
		if len(request.Tools) == 0 {
			return completedTextResponse("schedule"), nil
		}
		for _, message := range request.Messages {
			for _, part := range message.Parts {
				if part.ToolResult != nil && part.ToolResult.Name == name {
					return completedTextResponse("scheduled"), nil
				}
			}
		}
		call := chat.ToolCall{ID: "discover", Name: "search_tools", Arguments: `{"query":"select:` + name + `"}`}
		for _, definition := range request.Tools {
			if definition.Name == name {
				call = chat.ToolCall{ID: "create", Name: name, Arguments: `{"title":"model-owned schedule","instructions":"inspect progress","cron":"0 0 1 1 *"}`}
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
	cfg.ChatResolver = testChatResolver(client)
	cfg.ApprovalMode = approval.ModeYolo
	r := poMCPOpen(t, cfg)
	installed := poMCPActivate(r, filepath.Join(firstPartyPackages, "schedules"))
	server := &protocol.MCPServerID{Origin: protocol.MCPOrigin{Type: protocol.MCPOriginInstallation, InstallationID: installed.ID}, Name: "schedules"}
	r.poMCPTools(server, func(tools []protocol.MCPTool) bool { return len(tools) == 3 })
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	subscription := r.endpoint.Invoke(ctx, delivery.RuntimeSubscribe, protocol.RuntimeSubscribeRequest{Topics: []protocol.RuntimeTopic{protocol.TopicSchedulesChanged}}, delivery.Options{RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.ProtocolVersion}})
	if subscription.Failure != nil {
		t.Fatal(subscription.Failure)
	}
	changed := make(chan error, 1)
	go func() {
		for event, err := range subscription.Events {
			if err != nil {
				changed <- err
				return
			}
			if event.(protocol.RuntimeEvent).Type == protocol.RuntimeSchedulesChanged {
				changed <- nil
				return
			}
		}
		changed <- errors.New("schedule subscription ended before invalidation")
	}()
	session := r.must(delivery.SessionsCreate, protocol.CreateSessionRequest{Title: "Schedule from model"}, "session").(*protocol.Session)
	started := r.endpoint.Invoke(t.Context(), delivery.RunsStart, protocol.StartRunRequest{SessionID: session.ID, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "Create a recurring progress inspection."}}}, delivery.Options{RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.ProtocolVersion}})
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
	waitForRunEvents(t, collectRunEvents(events), "backend Schedule tool")
	page := r.must(delivery.SchedulesList, protocol.PageQuery{}, "").(*protocol.Page[protocol.Schedule])
	if len(page.Data) != 1 || page.Data[0].Title != "model-owned schedule" {
		t.Fatalf("model Tool did not reach the human fact owner: %+v", page.Data)
	}
	select {
	case err := <-changed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(lifecycleWaitBudget(t)):
		t.Fatal("model Tool left human Schedule reads stale")
	}
}

func TestMemoryServiceHasOneOwnerAndReenableRetainsItsPrivateFacts(t *testing.T) {
	cfg := poMCPConfig(t)
	r := poMCPOpen(t, cfg)
	source := filepath.Join(firstPartyPackages, "memory")
	first := poMCPActivate(r, source)
	server := func(id string) *protocol.MCPServerID {
		return &protocol.MCPServerID{Origin: protocol.MCPOrigin{Type: protocol.MCPOriginInstallation, InstallationID: id}, Name: "memory"}
	}
	r.poMCPTools(server(first.ID), func(tools []protocol.MCPTool) bool { return len(tools) == 1 })
	fact := r.must(delivery.AgentMemoryAdd, protocol.AgentMemoryAddRequest{Scope: protocol.AgentMemoryScopeUser, Content: "owned fact"}, "fact").(*protocol.AgentMemoryItem)
	second := r.must(delivery.PluginsInstall, protocol.InstallPluginRequest{Source: source}, "second-install").(*protocol.PluginInstallation)
	r.must(delivery.PluginsApprove, protocol.PluginReleaseRequest{InstallationID: second.ID, Digest: second.Selected.Digest}, "second-approve")
	invoke := func(method delivery.Name, input any, key string) delivery.Result {
		return r.endpoint.Invoke(t.Context(), method, input, delivery.Options{IdempotencyKey: key, RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.ProtocolVersion}})
	}
	conflict := invoke(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: second.ID, Enabled: true}, "second-enable")
	if !errors.Is(conflict.Failure, protocol.ErrInvalidParams) {
		t.Fatalf("second Memory owner=%v", conflict.Failure)
	}
	secondState := r.must(delivery.PluginsList, struct{}{}, "").(*protocol.Page[protocol.PluginInstallation])
	for _, installation := range secondState.Data {
		if installation.ID == second.ID && installation.State == protocol.PluginInstallationEnabled {
			t.Fatal("refused owner became enabled")
		}
	}
	reused := invoke(delivery.AgentMemoryAdd, protocol.AgentMemoryAddRequest{Scope: protocol.AgentMemoryScopeUser, Content: "different fact"}, "fact")
	if !errors.Is(reused.Failure, protocol.ErrIdempotencyConflict) {
		t.Fatalf("private command identity conflict=%v", reused.Failure)
	}
	r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: first.ID, Enabled: false}, "disable")
	unavailable := invoke(delivery.AgentMemoryList, protocol.AgentMemoryListRequest{Scope: protocol.AgentMemoryScopeUser}, "")
	if !errors.Is(unavailable.Failure, protocol.ErrPluginUnavailable) {
		t.Fatalf("withdrawn owner remained callable=%v", unavailable.Failure)
	}
	r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: first.ID, Enabled: true}, "reenable")
	r.poMCPTools(server(first.ID), func(tools []protocol.MCPTool) bool { return len(tools) == 1 })
	page := r.must(delivery.AgentMemoryList, protocol.AgentMemoryListRequest{Scope: protocol.AgentMemoryScopeUser}, "").(*protocol.Page[protocol.AgentMemoryItem])
	if len(page.Data) != 1 || page.Data[0] != *fact {
		t.Fatalf("reenable lost private facts=%+v", page)
	}
}
func TestScheduleManualCommandReplaysAcceptedRunAcrossDisableAndRestart(t *testing.T) {
	cfg := poMCPConfig(t)
	r := poMCPOpen(t, cfg)
	installed := poMCPActivate(r, filepath.Join(firstPartyPackages, "schedules"))
	server := &protocol.MCPServerID{Origin: protocol.MCPOrigin{Type: protocol.MCPOriginInstallation, InstallationID: installed.ID}, Name: "schedules"}
	ready := func(tools []protocol.MCPTool) bool { return len(tools) == 3 }
	r.poMCPTools(server, ready)
	saved := r.must(delivery.SchedulesCreate, protocol.CreateScheduleRequest{Title: "captured title", Instructions: "captured instructions", Cron: "@hourly", Workspace: &protocol.WorkspaceRef{Path: cfg.DefaultWorkspacePath}}, "create").(*protocol.Schedule)
	request := protocol.RunScheduleNowRequest{ID: saved.ID}
	accepted := r.must(delivery.SchedulesRunNow, request, "manual").(*protocol.RunScheduleNowResponse)
	r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: installed.ID, Enabled: false}, "disable-schedules")
	deadline := time.Now().Add(5 * time.Second)
	for {
		current := r.must(delivery.RunsGet, protocol.GetRunRequest{RunID: accepted.RunID}, "").(*protocol.RunRef)
		if current.Status == protocol.RunStatusFinished {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("accepted Run did not survive backend withdrawal: %+v", current)
		}
		time.Sleep(10 * time.Millisecond)
	}
	r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: installed.ID, Enabled: true}, "reenable-schedules")
	r.poMCPTools(server, ready)
	replay := r.must(delivery.SchedulesRunNow, request, "manual").(*protocol.RunScheduleNowResponse)
	if *replay != *accepted {
		t.Fatal("manual replay opened another Run")
	}
	r.close()
	r = poMCPOpen(t, cfg)
	replay = r.must(delivery.SchedulesRunNow, request, "manual").(*protocol.RunScheduleNowResponse)
	if *replay != *accepted {
		t.Fatal("restart reopened an accepted Run")
	}
	page := r.must(delivery.SchedulesList, protocol.PageQuery{}, "").(*protocol.Page[protocol.Schedule])
	if len(page.Data) != 1 || page.Data[0].Revision != saved.Revision+1 {
		t.Fatalf("acceptance replay advanced definition again=%+v", page.Data)
	}
}

func TestMemoryCommittedReplyLossReplaysFromItsSingleOwner(t *testing.T) {
	source := filepath.Join(t.TempDir(), "memory")
	if err := os.CopyFS(source, os.DirFS(filepath.Join(firstPartyPackages, "memory"))); err != nil {
		t.Fatal(err)
	}
	evidence := filepath.Join(t.TempDir(), "lost-response.json")
	build := exec.CommandContext(t.Context(), "go", "build", "-trimpath", "-ldflags=-s -w", "-o", filepath.Join(source, "bin", "response-loss"), "./testdata/review-response-loss")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build response-loss fixture: %s %v", output, err)
	}
	descriptor, err := json.Marshal(map[string]any{"$schema": "https://agent-plugins.org/schemas/1.0.0/mcp.schema.json", "mcpServers": map[string]any{"memory": map[string]any{"type": "stdio", "command": "./bin/response-loss", "args": []string{evidence, "./bin/memory", "flame.memory/add.v1"}}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "mcp.json"), descriptor, 0600); err != nil {
		t.Fatal(err)
	}
	cfg := poMCPConfig(t)
	r := poMCPOpen(t, cfg)
	installed := poMCPActivate(r, source)
	if len(installed.Selected.Diagnostics) != 0 {
		t.Fatalf("response-loss fixture admission=%+v", installed.Selected.Diagnostics)
	}
	server := protocol.MCPServerID{Origin: protocol.MCPOrigin{Type: protocol.MCPOriginInstallation, InstallationID: installed.ID}, Name: "memory"}
	r.poMCPTools(&server, func(tools []protocol.MCPTool) bool { return len(tools) == 1 })
	request := protocol.AgentMemoryAddRequest{Scope: protocol.AgentMemoryScopeUser, Content: "committed before transport loss"}
	lost := r.endpoint.Invoke(t.Context(), delivery.AgentMemoryAdd, request, delivery.Options{IdempotencyKey: "lost", RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.ProtocolVersion}})
	if lost.Failure == nil {
		t.Fatal("withheld reply appeared successful")
	}
	if _, err := os.Stat(evidence); err != nil {
		t.Fatalf("backend did not commit before response loss: %v", err)
	}
	r.must(delivery.MCPServersReconnect, protocol.MCPServerRequest{Server: server}, "reconnect")
	r.poMCPTools(&server, func(tools []protocol.MCPTool) bool { return len(tools) == 1 })
	replay := r.must(delivery.AgentMemoryAdd, request, "lost").(*protocol.AgentMemoryItem)
	page := r.must(delivery.AgentMemoryList, protocol.AgentMemoryListRequest{Scope: protocol.AgentMemoryScopeUser}, "").(*protocol.Page[protocol.AgentMemoryItem])
	if len(page.Data) != 1 || page.Data[0] != *replay {
		t.Fatalf("lost reply replay repeated the mutation=%+v", page.Data)
	}
}
