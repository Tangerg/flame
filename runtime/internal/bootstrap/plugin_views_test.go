package bootstrap

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/protocol"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient"
)

func TestPluginViewReadsUseInstalledBytesAndExistingTrajectory(t *testing.T) {
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
	source, err := filepath.Abs("../../../plugins/trajectory")
	if err != nil {
		t.Fatal(err)
	}
	installed := r.must(delivery.PluginsInstall, protocol.InstallPluginRequest{Source: source}, "install").(*protocol.PluginInstallation)
	if len(installed.Selected.Views) != 1 || len(installed.Selected.Diagnostics) != 0 {
		t.Fatalf("first-party admission: %+v", installed.Selected)
	}
	request := protocol.ReadPluginViewRequest{PluginReleaseRequest: protocol.PluginReleaseRequest{InstallationID: installed.ID, Digest: installed.Selected.Digest}, ViewID: "trajectory"}
	checkFailure := func(method delivery.Name, input any, want error) {
		t.Helper()
		result := r.endpoint.Invoke(t.Context(), method, input, delivery.Options{RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.ProtocolVersion}})
		if !errors.Is(result.Failure, want) {
			t.Fatalf("%s: %v, want %v", method, result.Failure, want)
		}
	}
	checkFailure(delivery.PluginsReadView, request, protocol.ErrPluginUnapproved)
	r.must(delivery.PluginsApprove, request.PluginReleaseRequest, "approve")
	r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: installed.ID, Enabled: true}, "enable")
	html := r.must(delivery.PluginsReadView, request, "").(*protocol.PluginViewResource)
	authored, err := os.ReadFile(filepath.Join(source, "views", "trajectory.html"))
	if err != nil {
		t.Fatal(err)
	}
	if html.HTML != string(authored) {
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
	read := protocol.ReadPluginTrajectoryRequest{ReadPluginViewRequest: request, ListSessionTrajectoryRequest: protocol.ListSessionTrajectoryRequest{SessionID: session.ID}}
	page := r.must(delivery.PluginsReadTrajectory, read, "").(*protocol.Page[protocol.TrajectoryEntry])
	canonical := r.must(delivery.SessionsTrajectory, protocol.ListSessionTrajectoryRequest{SessionID: session.ID}, "").(*protocol.Page[protocol.TrajectoryEntry])
	if !reflect.DeepEqual(page, canonical) {
		t.Fatal("plugin invented another trajectory projection")
	}
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
	checkFailure(delivery.PluginsReadTrajectory, descendants, protocol.ErrCapabilityNotNeg)
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
		observed := invoke(delivery.PluginsReadTrajectory, descendants)
		canonical := invoke(delivery.SessionsTrajectory, descendants.ListSessionTrajectoryRequest)
		if !reflect.DeepEqual(observed, canonical) || len(observed.Data) != 1 {
			t.Fatal("scoped plugin page differs from its canonical query")
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
			result := r.endpoint.Invoke(t.Context(), delivery.PluginsReadTrajectory, changedScope, options)
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
	missing.ViewID = "missing"
	checkFailure(delivery.PluginsReadView, missing, protocol.ErrPluginNotFound)
	stale := request
	stale.Digest = "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	checkFailure(delivery.PluginsReadView, stale, protocol.ErrPluginStale)
	r.close()
	r = poMCPOpen(t, cfg)
	if got := r.must(delivery.PluginsReadView, request, "").(*protocol.PluginViewResource); got.HTML != html.HTML {
		t.Fatal("restart reinterpreted the admitted view")
	}
	r.must(delivery.PluginsRevoke, protocol.PluginRequest{InstallationID: installed.ID}, "revoke")
	checkFailure(delivery.PluginsReadView, request, protocol.ErrPluginUnapproved)
	checkFailure(delivery.PluginsReadTrajectory, read, protocol.ErrPluginUnapproved)
}
