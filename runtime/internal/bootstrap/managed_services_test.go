package bootstrap

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"iter"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	pluginsdk "github.com/Tangerg/flame/packages/plugin-sdk/go"
	memorydomain "github.com/Tangerg/flame/plugins/memory/backend/domain"
	memoryrpc "github.com/Tangerg/flame/plugins/memory/backend/rpc"
	scheduleapp "github.com/Tangerg/flame/plugins/schedules/backend/application"
	scheduledomain "github.com/Tangerg/flame/plugins/schedules/backend/domain"
	schedulerpc "github.com/Tangerg/flame/plugins/schedules/backend/rpc"
	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	plugin "github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
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
	subscription := r.endpoint.Invoke(ctx, delivery.RuntimeSubscribe, protocol.RuntimeSubscribeRequest{Topics: []protocol.RuntimeTopic{protocol.TopicPluginDataChanged}}, delivery.Options{RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.Version}})
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
			if event.(protocol.RuntimeEvent).Type == protocol.RuntimePluginDataChanged {
				changed <- nil
				return
			}
		}
		changed <- errors.New("schedule subscription ended before invalidation")
	}()
	session := r.must(delivery.SessionsCreate, protocol.CreateSessionRequest{Title: "Schedule from model"}, "session").(*protocol.Session)
	started := r.endpoint.Invoke(t.Context(), delivery.RunsStart, protocol.StartRunRequest{SessionID: session.ID, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "Create a recurring progress inspection."}}}, delivery.Options{IdempotencyKey: "schedule-tool-start", RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.Version}})
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
	page := readSchedulePage(r, protocol.PageQuery{})
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

func TestMemoryServiceHasOneSelectedOwnerAndReenableRetainsItsPrivateFacts(t *testing.T) {
	cfg := poMCPConfig(t)
	r := poMCPOpen(t, cfg)
	source := filepath.Join(firstPartyPackages, "memory")
	first := poMCPActivate(r, source)
	server := func(id string) *protocol.MCPServerID {
		return &protocol.MCPServerID{Origin: protocol.MCPOrigin{Type: protocol.MCPOriginInstallation, InstallationID: id}, Name: "memory"}
	}
	r.poMCPTools(server(first.ID), func(tools []protocol.MCPTool) bool { return len(tools) == 1 })
	fact := mustMemoryAdd(r, memoryAddRequest(r, memoryrpc.AddInput{Scope: memorydomain.ScopeUser, Content: "owned fact"}), "fact")
	second := r.must(delivery.PluginsInstall, protocol.InstallPluginRequest{Source: source}, "second-install").(*protocol.PluginInstallation)
	invoke := func(method delivery.Name, input any, key string) delivery.Result {
		return r.endpoint.Invoke(t.Context(), method, input, delivery.Options{IdempotencyKey: key, RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.Version}})
	}
	enabled := invoke(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: second.ID, Enabled: true}, "second-enable")
	if enabled.Failure != nil || !enabled.Value.(*protocol.PluginInstallation).Enabled {
		t.Fatalf("second candidate was not enabled: %v", enabled.Failure)
	}
	choices := r.must(delivery.PluginsListProviders, struct{}{}, "").(*protocol.Page[protocol.PluginProviderSelection])
	if len(choices.Data) != 1 || choices.Data[0].InstallationID != first.ID {
		t.Fatalf("enabling another candidate changed the owner: %+v", choices.Data)
	}
	secondState := r.must(delivery.PluginsList, struct{}{}, "").(*protocol.Page[protocol.PluginInstallation])
	for _, installation := range secondState.Data {
		if installation.ID == second.ID && !installation.Enabled {
			t.Fatal("independent candidate lost enablement")
		}
	}
	reused := invoke(delivery.PluginsCall, memoryAddRequest(r, memoryrpc.AddInput{Scope: memorydomain.ScopeUser, Content: "different fact"}), "fact")
	if !errors.Is(reused.Failure, protocol.ErrIdempotencyConflict) {
		t.Fatalf("private command identity conflict=%v", reused.Failure)
	}
	r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: first.ID, Enabled: false}, "disable")
	unavailable := memoryQueryResult(r, memoryrpc.ListInput{Scope: memorydomain.ScopeUser})
	if !errors.Is(unavailable.Failure, protocol.ErrPluginUnavailable) {
		t.Fatalf("withdrawn owner remained callable=%v", unavailable.Failure)
	}
	r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: first.ID, Enabled: true}, "reenable")
	r.poMCPTools(server(first.ID), func(tools []protocol.MCPTool) bool { return len(tools) == 1 })
	page := readMemoryPage(r, memoryrpc.ListInput{Scope: memorydomain.ScopeUser})
	if len(page.Data) != 1 || !reflect.DeepEqual(page.Data[0], *fact) {
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
	saved := readScheduleCreate(r, scheduleCreateRequest(r, scheduledomain.Draft{Enabled: true, Title: "captured title", Instructions: "captured instructions", Cron: "@hourly", CWD: cfg.DefaultWorkspacePath}), "create")
	request := scheduleRunRequest(r, saved.ID)
	accepted := readScheduleRun(r, request, "manual")
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
	replay := readScheduleRun(r, request, "manual")
	if *replay != *accepted {
		t.Fatal("manual replay opened another Run")
	}
	r.close()
	r = poMCPOpen(t, cfg)
	r.poMCPTools(server, ready)
	replay = readScheduleRun(r, request, "manual")
	if *replay != *accepted {
		t.Fatal("restart reopened an accepted Run")
	}
	page := readSchedulePage(r, protocol.PageQuery{})
	if len(page.Data) != 1 || page.Data[0].Revision != saved.Revision+1 {
		t.Fatalf("acceptance replay advanced definition again=%+v", page.Data)
	}
}

func TestMemoryCommittedReplyLossReplaysFromItsSingleOwner(t *testing.T) {
	source, evidence := managedMessageLossPackage(t, "memory", "flame.memory/add.v1", "response")
	cfg := poMCPConfig(t)
	r := poMCPOpen(t, cfg)
	installed := poMCPActivate(r, source)
	if len(installed.Selected.Diagnostics) != 0 {
		t.Fatalf("response-loss fixture admission=%+v", installed.Selected.Diagnostics)
	}
	server := protocol.MCPServerID{Origin: protocol.MCPOrigin{Type: protocol.MCPOriginInstallation, InstallationID: installed.ID}, Name: "memory"}
	r.poMCPTools(&server, func(tools []protocol.MCPTool) bool { return len(tools) == 1 })
	changed := managedServiceChange(t, r, protocol.TopicPluginDataChanged)
	request := memoryAddRequest(r, memoryrpc.AddInput{Scope: memorydomain.ScopeUser, Content: "committed before transport loss"})
	lost := r.endpoint.Invoke(t.Context(), delivery.PluginsCall, request, delivery.Options{IdempotencyKey: "lost", RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.Version}})
	if lost.Failure == nil {
		t.Fatal("withheld reply appeared successful")
	}
	if _, err := os.Stat(evidence); err != nil {
		t.Fatalf("backend did not commit before response loss: %v", err)
	}
	commandID := operation.Identity(operation.Bind(t.Context(), cfg.Stores.IdempotencyNamespace.String(), "lost"))
	original, found, err := cfg.Stores.PluginCommands.Get(t.Context(), commandID)
	if err != nil || !found {
		t.Fatalf("original command binding missing: %t, %v", found, err)
	}
	contract := original.Snapshot().Contract
	var document capability.Descriptor
	if err := json.Unmarshal(contract.JSON(), &document); err != nil {
		t.Fatalf("original contract not captured: %v", err)
	}
	var add capability.MethodDescriptor
	for _, method := range document.Methods {
		if method.Name == "flame.memory/add.v1" {
			add = method
		}
	}
	if add.Kind != capability.Mutation || len(add.Input) == 0 || len(add.Output) == 0 {
		t.Fatalf("original contract did not capture flame.memory/add.v1: %+v", document.Methods)
	}
	requireManagedServiceChange(t, changed)
	installed = stageCommandReplacement(t, r, installed, source)
	assertUnresolvedInstallationIsHeld(t, r, installed)
	r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: installed.ID, Enabled: false}, "disable-memory")
	unavailable := memoryQueryResult(r, memoryrpc.ListInput{Scope: memorydomain.ScopeUser})
	if !errors.Is(unavailable.Failure, protocol.ErrPluginUnavailable) {
		t.Fatalf("disabled Memory remained callable: %v", unavailable.Failure)
	}
	r = reopenCommandRuntime(t, r, &cfg)
	restored, found, err := cfg.Stores.PluginCommands.Get(t.Context(), commandID)
	if err != nil || !found || restored.Snapshot().Contract != contract {
		t.Fatalf("restart changed original contract: %t, %v", found, err)
	}
	assertUnresolvedInstallationIsHeld(t, r, installed)
	pending := r.endpoint.Invoke(t.Context(), delivery.PluginsCall, request, delivery.Options{IdempotencyKey: "lost", RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.Version}})
	if pending.Failure == nil || !errors.Is(pending.Failure, protocol.ErrInternalError) {
		t.Fatalf("disabled original command became a definite result: %v", pending.Failure)
	}
	r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: installed.ID, Enabled: true}, "reenable-memory")
	r.poMCPTools(&server, func(tools []protocol.MCPTool) bool { return len(tools) == 1 })
	replay := mustMemoryAdd(r, request, "lost")
	page := readMemoryPage(r, memoryrpc.ListInput{Scope: memorydomain.ScopeUser})
	if len(page.Data) != 1 || !reflect.DeepEqual(page.Data[0], *replay) {
		t.Fatalf("lost reply replay repeated the mutation=%+v", page.Data)
	}
	observed, found, err := cfg.Stores.PluginCommands.Get(t.Context(), commandID)
	if err != nil || !found || observed.Snapshot().Contract != contract || observed.Snapshot().Phase != plugin.CommandObserved {
		t.Fatalf("receipt did not observe original contract: %t, %v", found, err)
	}
	r.must(delivery.PluginsConfigure, protocol.ConfigurePluginRequest{InstallationID: installed.ID, Digest: installed.Selected.Digest}, "configure-after-receipt")
}

func assertUnresolvedInstallationIsHeld(t *testing.T, r *poMCPRuntime, installed *protocol.PluginInstallation) {
	t.Helper()
	for _, action := range []struct {
		method delivery.Name
		input  any
	}{
		{delivery.PluginsUninstall, protocol.PluginRequest{InstallationID: installed.ID}},
		{delivery.PluginsConfigure, protocol.ConfigurePluginRequest{InstallationID: installed.ID, Digest: installed.Selected.Digest}},
	} {
		result := r.endpoint.Invoke(t.Context(), action.method, action.input, delivery.Options{RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.Version}})
		if !errors.Is(result.Failure, protocol.ErrPluginInUse) {
			t.Fatalf("unresolved command did not pin its installation for %s: %v", action.method, result.Failure)
		}
	}
	if installed.Staged != nil {
		result := r.endpoint.Invoke(t.Context(), delivery.PluginsSelect, protocol.PluginReleaseRequest{InstallationID: installed.ID, Digest: installed.Staged.Digest}, delivery.Options{RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.Version}})
		if !errors.Is(result.Failure, protocol.ErrPluginInUse) {
			t.Fatalf("unresolved command did not pin its release: %v", result.Failure)
		}
	}
}

func managedMessageLossPackage(t *testing.T, name, method, stage string) (string, string) {
	t.Helper()
	source := filepath.Join(t.TempDir(), name)
	if err := os.CopyFS(source, os.DirFS(filepath.Join(firstPartyPackages, name))); err != nil {
		t.Fatal(err)
	}
	evidence := filepath.Join(t.TempDir(), "lost-message.json")
	build := exec.CommandContext(t.Context(), "go", "build", "-trimpath", "-ldflags=-s -w", "-o", filepath.Join(source, "bin", "message-loss"), "./testdata/managed-message-loss")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build message-loss fixture: %s %v", output, err)
	}
	descriptor, err := json.Marshal(map[string]any{"$schema": "https://agent-plugins.org/schemas/1.0.0/mcp.schema.json", "mcpServers": map[string]any{name: map[string]any{"type": "stdio", "command": "./bin/message-loss", "args": []string{evidence, "./bin/" + name, method, "", stage}}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "mcp.json"), descriptor, 0600); err != nil {
		t.Fatal(err)
	}
	return source, evidence
}

func managedServiceChange(t *testing.T, r *poMCPRuntime, topic protocol.RuntimeTopic) <-chan error {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	subscription := r.endpoint.Invoke(ctx, delivery.RuntimeSubscribe, protocol.RuntimeSubscribeRequest{Topics: []protocol.RuntimeTopic{topic}}, delivery.Options{RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.Version}})
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
			value := event.(protocol.RuntimeEvent)
			if topic == protocol.TopicPluginDataChanged && value.Type == protocol.RuntimePluginDataChanged {
				changed <- nil
				return
			}
		}
		changed <- errors.New("service subscription ended before invalidation")
	}()
	return changed
}
func requireManagedServiceChange(t *testing.T, changed <-chan error) {
	t.Helper()
	select {
	case err := <-changed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(lifecycleWaitBudget(t)):
		t.Fatal("committed reply loss left human reads stale")
	}
}

func TestScheduleLostCreateReplyReplaysAfterRevocationAndRestart(t *testing.T) {
	source, evidence := managedMessageLossPackage(t, "schedules", schedulerpc.Create, "response")
	cfg := poMCPConfig(t)
	r := poMCPOpen(t, cfg)
	installed := poMCPActivate(r, source)
	server := &protocol.MCPServerID{Origin: protocol.MCPOrigin{Type: protocol.MCPOriginInstallation, InstallationID: installed.ID}, Name: "schedules"}
	ready := func(tools []protocol.MCPTool) bool { return len(tools) == 3 }
	r.poMCPTools(server, ready)
	changed := managedServiceChange(t, r, protocol.TopicPluginDataChanged)
	request := scheduleCreateRequest(r, scheduledomain.Draft{Enabled: true, Title: "captured schedule", Instructions: "inspect captured work", Cron: "0 0 1 1 *", CWD: cfg.DefaultWorkspacePath})
	lost := r.endpoint.Invoke(t.Context(), delivery.PluginsCall, request, delivery.Options{IdempotencyKey: "lost-create", RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.Version}})
	if lost.Failure == nil {
		t.Fatal("withheld create reply appeared successful")
	}
	requireManagedServiceChange(t, changed)
	raw, err := os.ReadFile(evidence)
	if err != nil {
		t.Fatal(err)
	}
	var captured struct {
		Response struct {
			Result pluginsdk.Result[scheduledomain.Snapshot] `json:"result"`
		} `json:"response"`
	}
	if err := json.Unmarshal(raw, &captured); err != nil {
		t.Fatal(err)
	}
	if captured.Response.Result.Value == nil {
		t.Fatal("create did not commit before response loss")
	}
	r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: installed.ID, Enabled: false}, "disable")
	r.close()
	r = poMCPOpen(t, cfg)
	r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: installed.ID, Enabled: true}, "reenable")
	r.poMCPTools(server, ready)
	replay := readScheduleCreate(r, request, "lost-create")
	page := readSchedulePage(r, protocol.PageQuery{})
	if len(page.Data) != 1 || !reflect.DeepEqual(page.Data[0], *replay) || replay.ID != captured.Response.Result.Value.ID {
		t.Fatalf("lost create repeated mutation: %+v", page.Data)
	}
}

func TestScheduleRunNowReplyLossRetainsAcceptanceAfterRestartAndSessionDeletion(t *testing.T) {
	source, evidence := managedMessageLossPackage(t, "schedules", schedulerpc.RunNow, "response")
	cfg := poMCPConfig(t)
	r := poMCPOpen(t, cfg)
	installed := poMCPActivate(r, source)
	server := &protocol.MCPServerID{Origin: protocol.MCPOrigin{Type: protocol.MCPOriginInstallation, InstallationID: installed.ID}, Name: "schedules"}
	ready := func(tools []protocol.MCPTool) bool { return len(tools) == 3 }
	r.poMCPTools(server, ready)
	saved := readScheduleCreate(r, scheduleCreateRequest(r, scheduledomain.Draft{Enabled: true, Title: "captured title", Instructions: "captured instructions", Cron: "0 0 1 1 *", CWD: cfg.DefaultWorkspacePath}), "create")
	request := scheduleRunRequest(r, saved.ID)
	lost := r.endpoint.Invoke(t.Context(), delivery.PluginsCall, request, delivery.Options{IdempotencyKey: "lost-manual", RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.Version}})
	if lost.Failure == nil {
		t.Fatal("withheld manual result appeared successful")
	}
	raw, err := os.ReadFile(evidence)
	if err != nil {
		t.Fatal(err)
	}
	var captured struct {
		Response struct {
			Result pluginsdk.Result[scheduleapp.RunReference] `json:"result"`
		} `json:"response"`
	}
	if err := json.Unmarshal(raw, &captured); err != nil {
		t.Fatal(err)
	}
	if captured.Response.Result.Value == nil {
		t.Fatal("manual acceptance did not commit before response loss")
	}
	accepted := scheduleapp.RunReference{SessionID: captured.Response.Result.Value.SessionID, RunID: captured.Response.Result.Value.RunID}
	r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: installed.ID, Enabled: false}, "disable")
	deadline := time.Now().Add(lifecycleWaitBudget(t))
	for {
		current := r.must(delivery.RunsGet, protocol.GetRunRequest{RunID: accepted.RunID}, "").(*protocol.RunRef)
		if current.Status == protocol.RunStatusFinished {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("accepted Run did not survive withdrawal: %+v", current)
		}
		time.Sleep(10 * time.Millisecond)
	}
	r.close()
	r = poMCPOpen(t, cfg)
	r.must(delivery.SessionsDelete, protocol.DeleteSessionRequest{SessionID: accepted.SessionID}, "delete-accepted")
	r.close()
	r = poMCPOpen(t, cfg)
	r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: installed.ID, Enabled: true}, "reenable")
	r.poMCPTools(server, ready)
	for range 2 {
		replay := readScheduleRun(r, request, "lost-manual")
		if *replay != accepted {
			t.Fatalf("manual replay changed acceptance: %+v", replay)
		}
	}
	page := readSchedulePage(r, protocol.PageQuery{})
	if len(page.Data) != 1 || page.Data[0].Revision != saved.Revision+1 {
		t.Fatalf("manual replay advanced definition: %+v", page.Data)
	}
	sessions := r.must(delivery.SessionsList, protocol.ListSessionsRequest{}, "").(*protocol.Page[protocol.Session])
	if len(sessions.Data) != 0 {
		t.Fatal("retired acceptance recreated its Session")
	}
}

func TestScheduleLostRunNowReplyRetainsCapturedWorkOnStartup(t *testing.T) {
	source, evidence := managedMessageLossPackage(t, "schedules", schedulerpc.RunNow, "response")
	cfg := poMCPConfig(t)
	r := poMCPOpen(t, cfg)
	installed := poMCPActivate(r, source)
	server := protocol.MCPServerID{Origin: protocol.MCPOrigin{Type: protocol.MCPOriginInstallation, InstallationID: installed.ID}, Name: "schedules"}
	ready := func(tools []protocol.MCPTool) bool { return len(tools) == 3 }
	r.poMCPTools(&server, ready)
	saved := readScheduleCreate(r, scheduleCreateRequest(r, scheduledomain.Draft{Enabled: true, Title: "captured title", Instructions: "captured instructions", Cron: "0 0 1 1 *", CWD: cfg.DefaultWorkspacePath}), "create")
	request := scheduleRunRequest(r, saved.ID)
	lost := r.endpoint.Invoke(t.Context(), delivery.PluginsCall, request, delivery.Options{IdempotencyKey: "lost-manual", RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.Version}})
	if lost.Failure == nil {
		t.Fatal("withheld manual reply appeared successful")
	}
	raw, err := os.ReadFile(evidence)
	if err != nil {
		t.Fatal(err)
	}
	var captured struct {
		Response struct {
			Result pluginsdk.Result[scheduleapp.RunReference] `json:"result"`
		} `json:"response"`
	}
	if err := json.Unmarshal(raw, &captured); err != nil {
		t.Fatal(err)
	}
	if captured.Response.Result.Value == nil {
		t.Fatal("manual acceptance did not commit")
	}
	pending := captured.Response.Result.Value
	r.must(delivery.MCPServersReconnect, protocol.MCPServerRequest{Server: server}, "reconnect")
	r.poMCPTools(&server, ready)
	title, instructions := "edited title", "edited instructions"
	r.must(delivery.PluginsCall, scheduleUpdateRequest(r, schedulerpc.UpdateInput{ID: saved.ID, ExpectedRevision: saved.Revision + 1, Title: &title, Instructions: &instructions}), "edit-after-capture")
	r.close()
	r = poMCPOpen(t, cfg)
	r.poMCPTools(&server, ready)
	workerCtx, cancelWorkers := context.WithCancel(t.Context())
	joins := r.host.application.startWorkers(workerCtx)
	t.Cleanup(func() { cancelWorkers(); <-joins.recovery })
	deadline := time.Now().Add(lifecycleWaitBudget(t))
	for {
		result := r.endpoint.Invoke(t.Context(), delivery.RunsGet, protocol.GetRunRequest{RunID: pending.RunID}, delivery.Options{RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.Version}})
		if result.Failure == nil {
			break
		}
		if !errors.Is(result.Failure, protocol.ErrRunNotFound) || time.Now().After(deadline) {
			t.Fatalf("startup did not recover private pending work: %v", result.Failure)
		}
		time.Sleep(10 * time.Millisecond)
	}
	session := r.must(delivery.SessionsGet, protocol.GetSessionRequest{SessionID: pending.SessionID}, "").(*protocol.Session)
	if session.Title != saved.Title {
		t.Fatalf("startup rewrote captured execution: %+v", session)
	}
	trajectory := r.must(delivery.SessionsTrajectory, protocol.ListSessionTrajectoryRequest{SessionID: pending.SessionID}, "").(*protocol.Page[protocol.TrajectoryEntry])
	capturedInput := false
	for _, entry := range trajectory.Data {
		if entry.Item == nil || entry.Item.Type != protocol.ItemTypeUserMessage {
			continue
		}
		if len(entry.Item.Content) != 1 || entry.Item.Content[0].Text != saved.Instructions {
			t.Fatalf("startup replaced captured instructions: %+v", entry.Item.Content)
		}
		capturedInput = true
	}
	if !capturedInput {
		t.Fatal("startup opening omitted captured instructions")
	}
	replay := readScheduleRun(r, request, "lost-manual")
	if replay.RunID != pending.RunID || replay.SessionID != pending.SessionID {
		t.Fatalf("startup recovery repeated admission: %+v", replay)
	}
	sessions := r.must(delivery.SessionsList, protocol.ListSessionsRequest{}, "").(*protocol.Page[protocol.Session])
	if len(sessions.Data) != 1 {
		t.Fatalf("startup recovery created extra Sessions: %+v", sessions.Data)
	}
}

func TestScheduleRunNowMissingReceiptRetainsUncertaintyWithoutCreatingRun(t *testing.T) {
	source, _ := managedMessageLossPackage(t, "schedules", schedulerpc.RunNow, "request")
	cfg := poMCPConfig(t)
	r := poMCPOpen(t, cfg)
	installed := poMCPActivate(r, source)
	server := &protocol.MCPServerID{Origin: protocol.MCPOrigin{Type: protocol.MCPOriginInstallation, InstallationID: installed.ID}, Name: "schedules"}
	ready := func(tools []protocol.MCPTool) bool { return len(tools) == 3 }
	r.poMCPTools(server, ready)
	saved := readScheduleCreate(r, scheduleCreateRequest(r, scheduledomain.Draft{Enabled: true, Title: "captured", Instructions: "inspect", Cron: "0 0 1 1 *"}), "create")
	request := scheduleRunRequest(r, saved.ID)
	options := delivery.Options{IdempotencyKey: "missing-receipt", RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.Version}}
	if outcome := r.endpoint.Invoke(t.Context(), delivery.PluginsCall, request, options); outcome.Failure == nil {
		t.Fatal("withheld manual request appeared successful")
	}
	r.must(delivery.MCPServersReconnect, protocol.MCPServerRequest{Server: *server}, "reconnect")
	r.poMCPTools(server, ready)
	if outcome := r.endpoint.Invoke(t.Context(), delivery.PluginsCall, request, options); !errors.Is(outcome.Failure, plugin.ErrCommandUnconfirmed) {
		t.Fatalf("missing receipt became permission to dispatch: %v", outcome.Failure)
	}
	sessions := r.must(delivery.SessionsList, protocol.ListSessionsRequest{}, "").(*protocol.Page[protocol.Session])
	if len(sessions.Data) != 0 {
		t.Fatal("receipt reconciliation originated a Run")
	}
	page := readSchedulePage(r, protocol.PageQuery{})
	if len(page.Data) != 1 || page.Data[0].Revision != saved.Revision {
		t.Fatal("missing receipt advanced the definition")
	}
}
