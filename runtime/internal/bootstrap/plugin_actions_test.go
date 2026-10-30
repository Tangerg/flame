package bootstrap

import (
	"errors"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
)

func TestSessionActionUsesCanonicalSessionRevisionAndDurableReplay(t *testing.T) {
	cfg := poMCPConfig(t)
	r := poMCPOpen(t, cfg)
	source, err := filepath.Abs(filepath.Join(firstPartyPackages, "trajectory"))
	if err != nil {
		t.Fatal(err)
	}
	installed := r.must(delivery.PluginsInstall, protocol.InstallPluginRequest{Source: source}, "install").(*protocol.PluginInstallation)
	index := slices.IndexFunc(installed.Selected.Components, func(component protocol.PluginClientComponent) bool {
		return component.ID == "rename-session"
	})
	if index < 0 || len(installed.Selected.Diagnostics) != 0 {
		t.Fatalf("action admission: %+v", installed.Selected)
	}
	component := installed.Selected.Components[index]
	if component.Scope != "session" || component.Placement != extension.ComponentPlacementSessionAction || component.Export != "RenameSession" {
		t.Fatalf("session action: %+v", component)
	}
	session := r.must(delivery.SessionsCreate, protocol.CreateSessionRequest{Title: "Original"}, "session").(*protocol.Session)
	title := "Renamed"
	request := protocol.UpdateSessionRequest{SessionID: session.ID, ExpectedRevision: session.Revision, Title: &title}
	resource := protocol.ReadPluginResourceRequest{OpenPluginResourcesRequest: protocol.OpenPluginResourcesRequest{PluginReleaseRequest: protocol.PluginReleaseRequest{InstallationID: installed.ID, Digest: installed.Selected.Digest}, ComponentID: component.ID}, Path: component.Entry}
	resourceRefused := func() {
		t.Helper()
		result := r.endpoint.Invoke(t.Context(), delivery.PluginsReadResource, resource, delivery.Options{RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.Version}})
		if !errors.Is(result.Failure, protocol.ErrPluginDisabled) {
			t.Fatalf("action resource after withdrawal: %v", result.Failure)
		}
	}
	resourceRefused()
	r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: installed.ID, Enabled: true}, "enable")
	r.must(delivery.PluginsReadResource, resource, "")
	refused := func(in protocol.UpdateSessionRequest, key string, want error) {
		t.Helper()
		result := r.endpoint.Invoke(t.Context(), delivery.SessionsUpdate, in, delivery.Options{RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.Version}, IdempotencyKey: key})
		if !errors.Is(result.Failure, want) {
			t.Fatalf("refusal: %v, want %v", result.Failure, want)
		}
	}
	saved := r.must(delivery.SessionsUpdate, request, "rename").(*protocol.Session)
	if saved.Title != title || saved.Revision != session.Revision+1 {
		t.Fatalf("canonical edit: %+v", saved)
	}
	replay := r.must(delivery.SessionsUpdate, request, "rename").(*protocol.Session)
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
	r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: installed.ID, Enabled: false}, "disable")
	resourceRefused()
	if got := r.must(delivery.SessionsUpdate, request, "rename").(*protocol.Session); !reflect.DeepEqual(saved, got) {
		t.Fatal("withdrawal changed the historical receipt")
	}
	r = reopenPluginRuntime(t, r, &cfg)
	if got := r.must(delivery.SessionsUpdate, request, "rename").(*protocol.Session); !reflect.DeepEqual(saved, got) {
		t.Fatal("restart did not retain the exact receipt")
	}
	resourceRefused()
	check()
}
