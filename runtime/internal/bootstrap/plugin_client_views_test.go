package bootstrap

import (
	"encoding/base64"
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
		{"icon-gallery", ""},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			r := poMCPOpen(t, poMCPConfig(t))
			source, err := filepath.Abs("../../../plugins/" + fixture.name)
			if err != nil {
				t.Fatal(err)
			}
			installed := r.must(delivery.PluginsInstall, protocol.InstallPluginRequest{Source: source}, "install").(*protocol.PluginInstallation)
			if len(installed.Selected.Diagnostics) != 0 {
				t.Fatalf("admission: %+v", installed.Selected)
			}
			view := protocol.ReadPluginViewRequest{PluginReleaseRequest: protocol.PluginReleaseRequest{InstallationID: installed.ID, Digest: installed.Selected.Digest}, ViewID: fixture.name}
			r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: installed.ID, Enabled: true}, "enable")
			var operation delivery.Name
			var request any
			var path, content string
			if fixture.kind == "" {
				if len(installed.Selected.Components) != 1 || len(installed.Selected.Views) != 0 || len(installed.Selected.Contributions) != 0 || installed.Selected.Components[0].Contribution != "" {
					t.Fatalf("component-only admission: %+v", installed.Selected)
				}
				component := installed.Selected.Components[0]
				path = component.Entry
				operation = delivery.PluginsReadResource
				request = protocol.ReadPluginResourceRequest{OpenPluginResourcesRequest: protocol.OpenPluginResourcesRequest{PluginReleaseRequest: view.PluginReleaseRequest, ComponentID: component.ID}, Path: path}
				resource := r.must(operation, request, "").(*protocol.PluginResource)
				body, err := base64.StdEncoding.DecodeString(resource.Body)
				if err != nil {
					t.Fatal(err)
				}
				content = string(body)
			} else {
				if len(installed.Selected.Views) != 1 || installed.Selected.Views[0].Type != fixture.kind {
					t.Fatalf("view admission: %+v", installed.Selected)
				}
				operation = delivery.PluginsReadView
				request = view
				path = filepath.Join("views", fixture.name+".html")
				content = r.must(operation, request, "").(*protocol.PluginViewResource).HTML
			}
			authored, err := os.ReadFile(filepath.Join(source, path))
			if err != nil {
				t.Fatal(err)
			}
			if content != string(authored) {
				t.Fatal("resource differs from the admitted release")
			}
			refused := r.endpoint.Invoke(t.Context(), delivery.PluginsReadUsage, protocol.ReadPluginUsageRequest{ReadPluginViewRequest: view}, delivery.Options{RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.ProtocolVersion}})
			if !errors.Is(refused.Failure, protocol.ErrPluginNotFound) {
				t.Fatalf("client view acquired Runtime query authority: %v", refused.Failure)
			}
			r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: installed.ID, Enabled: false}, "disable")
			refused = r.endpoint.Invoke(t.Context(), operation, request, delivery.Options{RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.ProtocolVersion}})
			if !errors.Is(refused.Failure, protocol.ErrPluginDisabled) {
				t.Fatalf("disabled resource admitted: %v", refused.Failure)
			}
		})
	}
}
