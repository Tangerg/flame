package bootstrap

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/protocol"
	"github.com/Tangerg/scope/core/chatclient"
)

func TestPluginViewReadsUseInstalledBytesAndExistingTrajectory(t *testing.T) {
	cfg := poMCPConfig(t)
	client, err := chatclient.New(newReplyStub("Observed response"), chatclient.Config{})
	if err != nil {
		t.Fatal(err)
	}
	cfg.ChatResolver = testChatResolver(client)
	r := poMCPOpen(t, cfg)
	source, err := filepath.Abs("../../../examples/plugins/trajectory")
	if err != nil {
		t.Fatal(err)
	}
	installed := r.must(delivery.PluginsInstall, protocol.InstallPluginRequest{Source: source}, "install").(*protocol.PluginInstallation)
	if len(installed.Selected.Views) != 1 || len(installed.Selected.Diagnostics) != 0 {
		t.Fatalf("example admission: %+v", installed.Selected)
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
	session := r.must(delivery.SessionsCreate, protocol.CreateSessionRequest{Title: "Inspection"}, "session").(*protocol.Session)
	_, events, err := r.api.StartRun(protocolLifecycleContext(t.Context()), protocol.StartRunRequest{SessionID: session.ID, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "Record an observation."}}})
	if err != nil {
		t.Fatal(err)
	}
	waitForRunEvents(t, collectRunEvents(events), "view source observations")
	read := protocol.ReadPluginTrajectoryRequest{ReadPluginViewRequest: request, SessionID: session.ID}
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
