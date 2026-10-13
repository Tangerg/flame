package bootstrap

import (
	"context"
	"encoding/base64"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"github.com/Tangerg/flame/runtime/capability"
	memorydomain "github.com/Tangerg/flame/runtime/internal/plugins/memory/domain"
	memoryrpc "github.com/Tangerg/flame/runtime/internal/plugins/memory/rpc"
	schedulerpc "github.com/Tangerg/flame/runtime/internal/plugins/schedules/rpc"
	"github.com/Tangerg/flame/runtime/jsoninput"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/delivery"
	scheduledomain "github.com/Tangerg/flame/runtime/internal/plugins/schedules/domain"
	"github.com/Tangerg/flame/runtime/protocol"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient"
)

func TestTrajectoryComponentReadsInstalledBytesAndCanonicalHistory(t *testing.T) {
	cfg := poMCPConfig(t)
	model := delegateRestartModel{chat.ModelFunc(func(_ context.Context, request *chat.Request) (*chat.Response, error) {
		for _, message := range request.Messages {
			if message.Role == chat.RoleTool || message.Role == chat.RoleUser && strings.Contains(message.Text(), "inspect child") {
				return completedTextResponse("Observed response"), nil
			}
		}
		message := chat.NewAssistantMessage(chat.NewToolCallPart(chat.ToolCall{ID: "delegate_inspection", Name: "delegate_task", Arguments: `{"summary":"Inspection","instructions":"inspect child"}`}))
		return chat.NewResponse(&chat.Output{Message: &message, FinishReason: chat.FinishReasonToolCalls}, &chat.ResponseMetadata{Model: "claude-test"})
	})}
	client, err := chatclient.New(model, chatclient.Config{})
	if err != nil {
		t.Fatal(err)
	}
	cfg.ChatResolver = testChatResolver(client)
	r := poMCPOpen(t, cfg)
	source := filepath.Join(firstPartyPackages, "trajectory")
	installed := r.must(delivery.PluginsInstall, protocol.InstallPluginRequest{Source: source}, "install").(*protocol.PluginInstallation)
	if len(installed.Selected.Components) != 1 || installed.Selected.Components[0].Scope != "session" || len(installed.Selected.Diagnostics) != 0 {
		t.Fatalf("first-party admission: %+v", installed.Selected)
	}
	request := protocol.ReadPluginResourceRequest{OpenPluginResourcesRequest: protocol.OpenPluginResourcesRequest{PluginReleaseRequest: protocol.PluginReleaseRequest{InstallationID: installed.ID, Digest: installed.Selected.Digest}, ComponentID: "trajectory"}, Path: installed.Selected.Components[0].Entry}
	checkFailure := func(method delivery.Name, input any, want error) {
		t.Helper()
		result := r.endpoint.Invoke(t.Context(), method, input, delivery.Options{RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.ProtocolVersion}})
		if !errors.Is(result.Failure, want) {
			t.Fatalf("%s: %v, want %v", method, result.Failure, want)
		}
	}
	checkFailure(delivery.PluginsReadResource, request, protocol.ErrPluginDisabled)
	r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: installed.ID, Enabled: true}, "enable")
	resource := r.must(delivery.PluginsReadResource, request, "").(*protocol.PluginResource)
	authored, err := os.ReadFile(filepath.Join(source, request.Path))
	if err != nil {
		t.Fatal(err)
	}
	body, err := base64.StdEncoding.DecodeString(resource.Body)
	if err != nil || !reflect.DeepEqual(body, authored) {
		t.Fatal("resource did not come from the exact admitted package")
	}
	session := r.must(delivery.SessionsCreate, protocol.CreateSessionRequest{Title: "Inspection", Workspace: &protocol.WorkspaceRef{Path: cfg.DefaultWorkspacePath}}, "session").(*protocol.Session)
	meta := protocol.RequestMeta{
		ProtocolVersion:    protocol.ProtocolVersion,
		ClientCapabilities: &protocol.ClientCapabilities{Features: map[string]protocol.FeaturePreference{protocol.FeatureSubagents: {Enabled: true}}},
	}
	started, events, err := r.api.StartRun(delivery.WithRequestMeta(t.Context(), meta), protocol.StartRunRequest{SessionID: session.ID, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "Record an observation."}}})
	if err != nil {
		t.Fatal(err)
	}
	waitForRunEvents(t, collectRunEvents(events), "view source observations")
	read := protocol.ListSessionTrajectoryRequest{SessionID: session.ID}
	page := r.must(delivery.SessionsTrajectory, read, "").(*protocol.Page[protocol.TrajectoryEntry])
	var models, runs, items int
	for _, entry := range page.Data {
		switch entry.Type {
		case protocol.TrajectoryEntryRun:
			runs++
		case protocol.TrajectoryEntryModel:
			models++
		case protocol.TrajectoryEntryItem:
			items++
		}
	}
	if models == 0 || runs == 0 || items == 0 {
		t.Fatalf("trajectory fixture lacks provider facts: %+v", page.Data)
	}
	descendants := read
	descendants.IncludeDescendants = true
	descendants.Limit = new(1)
	checkFailure(delivery.SessionsTrajectory, descendants, protocol.ErrCapabilityNotNeg)
	options := delivery.Options{RequestMeta: meta}
	invoke := func(method delivery.Name, input any) *protocol.Page[protocol.TrajectoryEntry] {
		t.Helper()
		result := r.endpoint.Invoke(t.Context(), method, input, options)
		if result.Failure != nil {
			t.Fatalf("%s: %v", method, result.Failure)
		}
		return result.Value.(*protocol.Page[protocol.TrajectoryEntry])
	}
	childObserved := false
	for {
		observed := invoke(delivery.SessionsTrajectory, descendants)
		if len(observed.Data) != 1 {
			t.Fatal("canonical trajectory did not preserve the requested page bound")
		}
		if entry := observed.Data[0]; entry.Run != nil && entry.Run.ParentRunID == started.RunID {
			childObserved = true
		}
		if observed.NextCursor == "" {
			break
		}
		if descendants.Cursor == "" {
			changedScope := descendants
			changedScope.IncludeDescendants = false
			changedScope.Cursor = observed.NextCursor
			result := r.endpoint.Invoke(t.Context(), delivery.SessionsTrajectory, changedScope, options)
			if !errors.Is(result.Failure, protocol.ErrInvalidParams) {
				t.Fatalf("cursor scope: %v", result.Failure)
			}
		}
		descendants.Cursor = observed.NextCursor
	}
	if !childObserved {
		t.Fatal("descendant scope omitted the child Run evidence")
	}
	missing := request
	missing.ComponentID = "missing"
	checkFailure(delivery.PluginsReadResource, missing, protocol.ErrPluginNotFound)
	stale := request
	stale.Digest = "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	checkFailure(delivery.PluginsReadResource, stale, protocol.ErrPluginStale)
	r.close()
	r = poMCPOpen(t, cfg)
	if got := r.must(delivery.PluginsReadResource, request, "").(*protocol.PluginResource); *got != *resource {
		t.Fatal("restart reinterpreted the admitted component bytes")
	}
	r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: installed.ID, Enabled: false}, "disable")
	checkFailure(delivery.PluginsReadResource, request, protocol.ErrPluginDisabled)
	if got := r.must(delivery.SessionsTrajectory, read, "").(*protocol.Page[protocol.TrajectoryEntry]); !reflect.DeepEqual(got, page) {
		t.Fatal("disabling a rendering component changed retained trajectory evidence")
	}
}

func TestMemoryComponentUsesInstalledPublicOperationsAndPreservesPrivateFacts(t *testing.T) {
	cfg := poMCPConfig(t)
	r := poMCPOpen(t, cfg)
	source := filepath.Join(firstPartyPackages, "memory")
	installed := r.must(delivery.PluginsInstall, protocol.InstallPluginRequest{Source: source}, "install-memory").(*protocol.PluginInstallation)
	if len(installed.Selected.Diagnostics) != 0 || len(installed.Selected.Components) != 1 || installed.Selected.Components[0].Scope != "client" {
		t.Fatalf("memory admission: %+v", installed.Selected)
	}
	resourceRequest := protocol.ReadPluginResourceRequest{OpenPluginResourcesRequest: protocol.OpenPluginResourcesRequest{PluginReleaseRequest: protocol.PluginReleaseRequest{InstallationID: installed.ID, Digest: installed.Selected.Digest}, ComponentID: "memory"}, Path: installed.Selected.Components[0].Entry}
	r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: installed.ID, Enabled: true}, "enable-memory")
	r.must(delivery.PluginsSelectProvider, protocol.SelectPluginProviderRequest{Role: "memory", InstallationID: installed.ID, Contribution: "memory-provider"}, "select-memory")
	resource := r.must(delivery.PluginsReadResource, resourceRequest, "").(*protocol.PluginResource)
	authored, err := os.ReadFile(filepath.Join(source, resourceRequest.Path))
	body, decodeErr := base64.StdEncoding.DecodeString(resource.Body)
	if err != nil || decodeErr != nil || !reflect.DeepEqual(body, authored) {
		t.Fatalf("memory resource differs from admitted bytes: %v", err)
	}
	r.poMCPTools(&protocol.MCPServerID{Origin: protocol.MCPOrigin{Type: protocol.MCPOriginInstallation, InstallationID: installed.ID}, Name: "memory"}, func(tools []protocol.MCPTool) bool { return len(tools) == 1 })
	for _, content := range []string{"first fact", "second fact"} {
		r.must(delivery.PluginsCall, memoryAddRequest(r, memoryrpc.AddInput{Scope: memorydomain.ScopeUser, Content: content}), content)
	}
	contracts, err := memoryrpc.NewContracts()
	if err != nil {
		t.Fatal(err)
	}
	query := memoryrpc.ListInput{Scope: memorydomain.ScopeUser, Limit: new(1)}
	for {
		input, err := jsoninput.Encode(memoryrpc.ListInput{Scope: memorydomain.ScopeUser, Cursor: query.Cursor, Limit: query.Limit})
		if err != nil {
			t.Fatal(err)
		}
		request := protocol.CallPluginRequest{InstallationID: installed.ID, Digest: installed.Selected.Digest, Contribution: "memory-provider", Contract: string(contracts.Capability.Identity().JSON()), Method: memoryrpc.List, Input: input.JSON(), Target: capability.Target{Scope: "installation", Reference: installed.ID}}
		result := r.must(delivery.PluginsCall, request, "").(*protocol.CallPluginResponse)
		if result.Failure != nil {
			t.Fatalf("installed list refusal: %+v", result.Failure)
		}
		var page memoryrpc.Page
		if err := json.Unmarshal(result.Output, &page); err != nil {
			t.Fatal(err)
		}
		canonical := readMemoryPage(r, query)
		if len(page.Rows) != 1 || len(canonical.Data) != 1 || page.Rows[0].ID != canonical.Data[0].ID || page.Rows[0].Content != canonical.Data[0].Content || page.NextCursor != canonical.NextCursor {
			t.Fatal("installed Memory operation differs from the private owner's canonical projection")
		}
		if page.NextCursor == "" {
			break
		}
		query.Cursor = page.NextCursor
	}
	r.must(delivery.PluginsUninstall, protocol.PluginRequest{InstallationID: installed.ID}, "uninstall-memory")
	result := memoryQueryResult(r, memoryrpc.ListInput{Scope: memorydomain.ScopeUser})
	if !errors.Is(result.Failure, protocol.ErrPluginUnavailable) {
		t.Fatalf("uninstalled Memory owner=%v", result.Failure)
	}
	retained := filepath.Join(cfg.Stores.DataDirectory, "plugins", "data", installed.ID, "memory.db")
	if _, err := os.Stat(retained); err != nil {
		t.Fatalf("uninstall discarded backend data: %v", err)
	}

}

func TestScheduleComponentUsesOwnBackendAndWithdrawalPreservesPrivateData(t *testing.T) {
	cfg := poMCPConfig(t)
	r := poMCPOpen(t, cfg)
	source := filepath.Join(firstPartyPackages, "schedules")
	installed := r.must(delivery.PluginsInstall, protocol.InstallPluginRequest{Source: source}, "install-schedules").(*protocol.PluginInstallation)
	if len(installed.Selected.Diagnostics) != 0 || len(installed.Selected.Components) != 1 || installed.Selected.Components[0].Contribution != "schedules-provider" {
		t.Fatalf("schedule admission: %+v", installed.Selected)
	}
	component := installed.Selected.Components[0]
	resource := protocol.ReadPluginResourceRequest{OpenPluginResourcesRequest: protocol.OpenPluginResourcesRequest{PluginReleaseRequest: protocol.PluginReleaseRequest{InstallationID: installed.ID, Digest: installed.Selected.Digest}, ComponentID: component.ID}, Path: component.Entry}
	r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: installed.ID, Enabled: true}, "enable-schedules")
	r.must(delivery.PluginsSelectProvider, protocol.SelectPluginProviderRequest{Role: "schedules", InstallationID: installed.ID, Contribution: "schedules-provider"}, "select-schedules")
	projection := r.must(delivery.PluginsReadResource, resource, "").(*protocol.PluginResource)
	body, err := base64.StdEncoding.DecodeString(projection.Body)
	authored, readErr := os.ReadFile(filepath.Join(source, component.Entry))
	if err != nil || readErr != nil || string(body) != string(authored) {
		t.Fatalf("schedule component bytes: %v, %v", err, readErr)
	}
	r.poMCPTools(&protocol.MCPServerID{Origin: protocol.MCPOrigin{Type: protocol.MCPOriginInstallation, InstallationID: installed.ID}, Name: "schedules"}, func(tools []protocol.MCPTool) bool { return len(tools) == 3 })
	for index, cron := range []string{"0 * * * *", "0 9 * * *"} {
		readScheduleCreate(r, scheduleCreateRequest(r, scheduledomain.Draft{Enabled: true, Title: "Inspect", Instructions: "Inspect current work", Cron: cron}), fmt.Sprintf("create-%d", index))
	}
	contracts, err := schedulerpc.NewContracts()
	if err != nil {
		t.Fatal(err)
	}
	query := protocol.PageQuery{Limit: new(1)}
	seen := map[string]bool{}
	for {
		input, err := json.Marshal(schedulerpc.ListInput{Cursor: query.Cursor, Limit: query.Limit})
		if err != nil {
			t.Fatal(err)
		}
		installedReply := r.must(delivery.PluginsCall, protocol.CallPluginRequest{InstallationID: installed.ID, Digest: installed.Selected.Digest, Contribution: "schedules-provider", Contract: string(contracts.Capability.Identity().JSON()), Method: schedulerpc.List, Target: capability.Target{Scope: "installation", Reference: installed.ID}, Input: input}, "").(*protocol.CallPluginResponse)
		var page schedulerpc.Page
		if err := json.Unmarshal(installedReply.Output, &page); err != nil {
			t.Fatal(err)
		}
		if len(page.Rows) != 1 || page.Rows[0].Title != "Inspect" || page.Rows[0].Instructions != "Inspect current work" || page.Rows[0].Revision != 1 {
			t.Fatal("installed schedule query lost accepted owner data")
		}
		if seen[page.Rows[0].ID] {
			t.Fatal("installed schedule page repeated an ID")
		}
		seen[page.Rows[0].ID] = true
		if page.NextCursor == "" {
			break
		}
		query.Cursor = page.NextCursor
	}
	if len(seen) != 2 {
		t.Fatalf("installed schedule traversal omitted accepted rows: %v", seen)
	}

	r.close()
	r = poMCPOpen(t, cfg)
	restarted := r.must(delivery.PluginsList, struct{}{}, "").(*protocol.Page[protocol.PluginInstallation])
	if !reflect.DeepEqual(restarted.Data[0].Selected.Components, installed.Selected.Components) {
		t.Fatal("restart changed admitted components")
	}
	r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: installed.ID, Enabled: false}, "disable-schedules")
	refused := r.endpoint.Invoke(t.Context(), delivery.PluginsReadResource, resource, delivery.Options{RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.ProtocolVersion}})
	if !errors.Is(refused.Failure, protocol.ErrPluginDisabled) {
		t.Fatalf("disabled schedule page: %v", refused.Failure)
	}
	r.must(delivery.PluginsUninstall, protocol.PluginRequest{InstallationID: installed.ID}, "uninstall-schedules")
	result := r.endpoint.Invoke(t.Context(), delivery.PluginsCall, protocol.CallPluginRequest{InstallationID: installed.ID, Digest: installed.Selected.Digest, Contribution: "schedules-provider", Contract: string(contracts.Capability.Identity().JSON()), Method: schedulerpc.List, Target: capability.Target{Scope: "installation", Reference: installed.ID}, Input: []byte(`{"Cursor":""}`)}, delivery.Options{RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.ProtocolVersion}})
	if !errors.Is(result.Failure, protocol.ErrPluginNotFound) {
		t.Fatalf("uninstalled Schedule owner=%v", result.Failure)
	}
	retained := filepath.Join(cfg.Stores.DataDirectory, "plugins", "data", installed.ID, "schedules.db")
	if _, err := os.Stat(retained); err != nil {
		t.Fatalf("uninstall discarded backend data: %v", err)
	}

}

func TestUsageComponentUsesCanonicalSummaryAndUninstallPreservesUsage(t *testing.T) {
	cfg := poMCPConfig(t)
	model := delegateRestartModel{chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
		message := chat.NewAssistantMessage(chat.NewTextPart("Recorded zero usage"))
		return chat.NewResponse(&chat.Output{Message: &message, FinishReason: chat.FinishReasonStop}, &chat.ResponseMetadata{Model: "claude-test", Usage: &chat.Usage{}})
	})}
	client, err := chatclient.New(model, chatclient.Config{})
	if err != nil {
		t.Fatal(err)
	}
	cfg.ChatResolver = testChatResolver(client)
	r := poMCPOpen(t, cfg)
	source, err := filepath.Abs("../../../plugins/usage")
	if err != nil {
		t.Fatal(err)
	}
	installed := r.must(delivery.PluginsInstall, protocol.InstallPluginRequest{Source: source}, "install-usage").(*protocol.PluginInstallation)
	if len(installed.Selected.Diagnostics) != 0 || len(installed.Selected.Components) != 1 {
		t.Fatalf("usage admission: %+v", installed.Selected)
	}
	release := protocol.PluginReleaseRequest{InstallationID: installed.ID, Digest: installed.Selected.Digest}
	component := installed.Selected.Components[0]
	resourceRequest := protocol.ReadPluginResourceRequest{OpenPluginResourcesRequest: protocol.OpenPluginResourcesRequest{PluginReleaseRequest: release, ComponentID: component.ID}, Path: component.Entry}
	read := protocol.UsageSummaryRequest{}
	refused := func(input any, method delivery.Name, want error) {
		t.Helper()
		result := r.endpoint.Invoke(t.Context(), method, input, delivery.Options{RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.ProtocolVersion}})
		if !errors.Is(result.Failure, want) {
			t.Fatalf("%s: %v, want %v", method, result.Failure, want)
		}
	}
	refused(resourceRequest, delivery.PluginsReadResource, protocol.ErrPluginDisabled)
	r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: installed.ID, Enabled: true}, "enable-usage")
	resource := r.must(delivery.PluginsReadResource, resourceRequest, "").(*protocol.PluginResource)
	body, err := base64.StdEncoding.DecodeString(resource.Body)
	if err != nil {
		t.Fatal(err)
	}
	authored, err := os.ReadFile(filepath.Join(source, component.Entry))
	if err != nil || string(body) != string(authored) {
		t.Fatalf("usage resource differs from admitted bytes: %v", err)
	}
	if empty := r.must(delivery.UsageSummary, read, "").(*protocol.UsageSummary); empty.Runs != 0 {
		t.Fatalf("empty usage invented runs: %+v", empty)
	}
	session := r.must(delivery.SessionsCreate, protocol.CreateSessionRequest{Title: "Reported zero"}, "usage-session").(*protocol.Session)
	_, events, err := r.api.StartRun(t.Context(), protocol.StartRunRequest{SessionID: session.ID, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "Record zero usage."}}})
	if err != nil {
		t.Fatal(err)
	}
	waitForRunEvents(t, collectRunEvents(events), "usage source record")
	var recorded *protocol.UsageSummary
	for _, days := range []*int{nil, new(7), new(30)} {
		read.SinceDays = days
		recorded = r.must(delivery.UsageSummary, read, "").(*protocol.UsageSummary)
		if recorded.Runs != 1 || recorded.Sessions != 1 || recorded.Total.InputTokens != 0 || recorded.Total.CostUSD != nil {
			t.Fatalf("usage report differs from recorded zero/unpriced usage: %+v", recorded)
		}
	}
	invalid := read
	invalid.SinceDays = new(0)
	refused(invalid, delivery.UsageSummary, protocol.ErrInvalidParams)
	r.close()
	r = poMCPOpen(t, cfg)
	if restarted := r.must(delivery.UsageSummary, read, "").(*protocol.UsageSummary); !reflect.DeepEqual(restarted, recorded) {
		t.Fatal("restart changed the recorded report")
	}
	r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: installed.ID, Enabled: false}, "disable-usage")
	refused(resourceRequest, delivery.PluginsReadResource, protocol.ErrPluginDisabled)
	r.must(delivery.PluginsUninstall, protocol.PluginRequest{InstallationID: installed.ID}, "uninstall-usage")
	if canonical := r.must(delivery.UsageSummary, read, "").(*protocol.UsageSummary); !reflect.DeepEqual(canonical, recorded) {
		t.Fatal("uninstall changed accounting records")
	}
}
