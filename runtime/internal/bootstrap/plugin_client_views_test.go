package bootstrap

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/protocol"
)

func TestClientPresentationPackagesAdmitResourcesWithoutRuntimeQueries(t *testing.T) {
	for _, fixture := range []struct {
		name string
		kind protocol.PluginViewType
	}{
		{"diagnostics", protocol.PluginViewClientDiagnostics},
		{"icon-gallery", protocol.PluginViewStatic},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			r := poMCPOpen(t, poMCPConfig(t))
			source, err := filepath.Abs("../../../plugins/" + fixture.name)
			if err != nil {
				t.Fatal(err)
			}
			installed := r.must(delivery.PluginsInstall, protocol.InstallPluginRequest{Source: source}, "install").(*protocol.PluginInstallation)
			if len(installed.Selected.Diagnostics) != 0 || len(installed.Selected.Views) != 1 || installed.Selected.Views[0].Type != fixture.kind {
				t.Fatalf("admission: %+v", installed.Selected)
			}
			view := protocol.ReadPluginViewRequest{PluginReleaseRequest: protocol.PluginReleaseRequest{InstallationID: installed.ID, Digest: installed.Selected.Digest}, ViewID: fixture.name}
			r.must(delivery.PluginsApprove, view.PluginReleaseRequest, "approve")
			r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: installed.ID, Enabled: true}, "enable")
			resource := r.must(delivery.PluginsReadView, view, "").(*protocol.PluginViewResource)
			authored, err := os.ReadFile(filepath.Join(source, "views", fixture.name+".html"))
			if err != nil {
				t.Fatal(err)
			}
			if resource.HTML != string(authored) {
				t.Fatal("resource differs from the admitted release")
			}
			refused := r.endpoint.Invoke(t.Context(), delivery.PluginsReadUsage, protocol.ReadPluginUsageRequest{ReadPluginViewRequest: view}, delivery.Options{RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.ProtocolVersion}})
			if !errors.Is(refused.Failure, protocol.ErrPluginNotFound) {
				t.Fatalf("client view acquired Runtime query authority: %v", refused.Failure)
			}
			r.must(delivery.PluginsRevoke, protocol.PluginRequest{InstallationID: installed.ID}, "revoke")
			refused = r.endpoint.Invoke(t.Context(), delivery.PluginsReadView, view, delivery.Options{RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.ProtocolVersion}})
			if !errors.Is(refused.Failure, protocol.ErrPluginUnapproved) {
				t.Fatalf("revoked resource admitted: %v", refused.Failure)
			}
		})
	}
}
