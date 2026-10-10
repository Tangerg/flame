package bootstrap

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/protocol"
)

func TestPluginRenameUsesCanonicalSessionRevisionAndDurableReplay(t *testing.T) {
	cfg := poMCPConfig(t)
	r := poMCPOpen(t, cfg)
	source, err := filepath.Abs("../../../plugins/trajectory")
	if err != nil {
		t.Fatal(err)
	}
	installed := r.must(delivery.PluginsInstall, protocol.InstallPluginRequest{Source: source}, "install").(*protocol.PluginInstallation)
	if len(installed.Selected.Actions) != 1 || len(installed.Selected.Diagnostics) != 0 {
		t.Fatalf("action admission: %+v", installed.Selected)
	}
	session := r.must(delivery.SessionsCreate, protocol.CreateSessionRequest{Title: "Original"}, "session").(*protocol.Session)
	title := "Renamed"
	request := protocol.RenamePluginSessionRequest{PluginReleaseRequest: protocol.PluginReleaseRequest{InstallationID: installed.ID, Digest: installed.Selected.Digest}, ActionID: "rename-session", Update: protocol.SessionTitleEdit{SessionID: session.ID, ExpectedRevision: session.Revision, Title: title}}
	refused := func(in protocol.RenamePluginSessionRequest, key string, want error) {
		t.Helper()
		result := r.endpoint.Invoke(t.Context(), delivery.PluginsRenameSession, in, delivery.Options{RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.ProtocolVersion}, IdempotencyKey: key})
		if !errors.Is(result.Failure, want) {
			t.Fatalf("refusal: %v, want %v", result.Failure, want)
		}
	}
	refused(request, "unapproved", protocol.ErrPluginUnapproved)
	r.must(delivery.PluginsApprove, request.PluginReleaseRequest, "approve")
	r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: installed.ID, Enabled: true}, "enable")
	missing := request
	missing.ActionID = "missing"
	refused(missing, "missing", protocol.ErrPluginNotFound)
	stale := request
	stale.Digest = "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	refused(stale, "stale", protocol.ErrPluginStale)
	saved := r.must(delivery.PluginsRenameSession, request, "rename").(*protocol.Session)
	if saved.Title != title || saved.Revision != session.Revision+1 {
		t.Fatalf("canonical edit: %+v", saved)
	}
	replay := r.must(delivery.PluginsRenameSession, request, "rename").(*protocol.Session)
	if !reflect.DeepEqual(saved, replay) {
		t.Fatal("replay advanced the Session again")
	}
	refused(request, "conflict", protocol.ErrRevisionConflict)
	check := func() {
		t.Helper()
		current := r.must(delivery.SessionsGet, protocol.GetSessionRequest{SessionID: session.ID}, "").(*protocol.Session)
		if !reflect.DeepEqual(saved, current) {
			t.Fatalf("Session owner differs: %+v", current)
		}
	}
	check()
	r.must(delivery.PluginsRevoke, protocol.PluginRequest{InstallationID: installed.ID}, "revoke")
	fresh := request
	fresh.Update.ExpectedRevision = saved.Revision
	refused(fresh, "withdrawn", protocol.ErrPluginUnapproved)
	if got := r.must(delivery.PluginsRenameSession, request, "rename").(*protocol.Session); !reflect.DeepEqual(saved, got) {
		t.Fatal("withdrawal changed the historical receipt")
	}
	r.close()
	r = poMCPOpen(t, cfg)
	if got := r.must(delivery.PluginsRenameSession, request, "rename").(*protocol.Session); !reflect.DeepEqual(saved, got) {
		t.Fatal("restart did not retain the exact receipt")
	}
	refused(fresh, "withdrawn-restart", protocol.ErrPluginUnapproved)
	check()
}
