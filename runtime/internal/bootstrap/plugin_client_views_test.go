package bootstrap

import (
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
)

func TestClientPresentationPackagesAdmitComponentsWithoutBackend(t *testing.T) {
	for _, name := range []string{"diagnostics", "icon-gallery", "skills"} {
		t.Run(name, func(t *testing.T) {
			r := poMCPOpen(t, poMCPConfig(t))
			source := filepath.Join(firstPartyPackages, name)
			installed := r.must(delivery.PluginsInstall, protocol.InstallPluginRequest{Source: source}, "install").(*protocol.PluginInstallation)
			if len(installed.Selected.Diagnostics) != 0 {
				t.Fatalf("admission: %+v", installed.Selected)
			}
			release := protocol.PluginReleaseRequest{InstallationID: installed.ID, Digest: installed.Selected.Digest}
			r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: installed.ID, Enabled: true}, "enable")
			if len(installed.Selected.Components) != 1 || len(installed.Selected.Contributions) != 0 || installed.Selected.Components[0].Contribution != "" {
				t.Fatalf("component-only admission: %+v", installed.Selected)
			}
			component := installed.Selected.Components[0]
			path := component.Entry
			operation := delivery.PluginsReadResource
			request := protocol.ReadPluginResourceRequest{OpenPluginResourcesRequest: protocol.OpenPluginResourcesRequest{PluginReleaseRequest: release, ComponentID: component.ID}, Path: path}
			resource := r.must(operation, request, "").(*protocol.PluginResource)
			body, err := base64.StdEncoding.DecodeString(resource.Body)
			if err != nil {
				t.Fatal(err)
			}
			content := string(body)
			authored, err := os.ReadFile(filepath.Join(source, path))
			if err != nil {
				t.Fatal(err)
			}
			if content != string(authored) {
				t.Fatal("resource differs from the admitted release")
			}
			r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: installed.ID, Enabled: false}, "disable")
			refused := r.endpoint.Invoke(t.Context(), operation, request, delivery.Options{RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.Version}})
			if !errors.Is(refused.Failure, protocol.ErrPluginDisabled) {
				t.Fatalf("disabled resource admitted: %v", refused.Failure)
			}
		})
	}
}
