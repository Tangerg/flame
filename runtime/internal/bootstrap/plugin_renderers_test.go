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

func TestOptionalRendererUsesSelectedImmutableBytes(t *testing.T) {
	r := poMCPOpen(t, poMCPConfig(t))
	source, err := filepath.Abs("../../../plugins/tool-results")
	if err != nil {
		t.Fatal(err)
	}
	installed := r.must(delivery.PluginsInstall, protocol.InstallPluginRequest{Source: source}, "install-renderer").(*protocol.PluginInstallation)
	if len(installed.Selected.Diagnostics) != 0 {
		t.Fatalf("admission: %+v", installed.Selected.Diagnostics)
	}
	var found bool
	for _, renderer := range installed.Selected.Renderers {
		if renderer.ID == "grep" {
			found = renderer.Selector.Type == protocol.PluginRendererToolName && renderer.Selector.Name == "grep"
		}
	}
	if !found {
		t.Fatal("recorded grep consumer was not admitted")
	}
	request := protocol.ReadPluginRendererRequest{PluginReleaseRequest: protocol.PluginReleaseRequest{InstallationID: installed.ID, Digest: installed.Selected.Digest}, RendererID: "grep"}
	refused := func(input protocol.ReadPluginRendererRequest, want error) {
		t.Helper()
		result := r.endpoint.Invoke(t.Context(), delivery.PluginsReadRenderer, input, delivery.Options{RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.ProtocolVersion}})
		if !errors.Is(result.Failure, want) {
			t.Fatalf("read: %v, want %v", result.Failure, want)
		}
	}
	refused(request, protocol.ErrPluginDisabled)
	r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: installed.ID, Enabled: true}, "enable-renderer")
	resource := r.must(delivery.PluginsReadRenderer, request, "").(*protocol.PluginViewResource)
	authored, err := os.ReadFile(filepath.Join(source, "renderers", "result.html"))
	if err != nil {
		t.Fatal(err)
	}
	if resource.HTML != string(authored) {
		t.Fatal("renderer did not use the admitted bytes")
	}
	wrong := request
	wrong.RendererID = "../result.html"
	refused(wrong, protocol.ErrInvalidParams)
	wrong.RendererID = "unknown"
	refused(wrong, protocol.ErrPluginNotFound)
	wrong = request
	wrong.Digest = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	refused(wrong, protocol.ErrPluginStale)
	r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: installed.ID, Enabled: false}, "disable-renderer")
	refused(request, protocol.ErrPluginDisabled)
}

func TestFileComponentUsesSelectedImmutableBytes(t *testing.T) {
	r := poMCPOpen(t, poMCPConfig(t))
	source, err := filepath.Abs("../../../plugins/file-renderers")
	if err != nil {
		t.Fatal(err)
	}
	installed := r.must(delivery.PluginsInstall, protocol.InstallPluginRequest{Source: source}, "install-file-component").(*protocol.PluginInstallation)
	if len(installed.Selected.Diagnostics) != 0 || len(installed.Selected.Components) != 3 || len(installed.Selected.Renderers) != 0 {
		t.Fatalf("file component admission: %+v", installed.Selected)
	}
	r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: installed.ID, Enabled: true}, "enable-file-component")
	for _, component := range installed.Selected.Components {
		if component.Scope != "file" || component.Placement != "fileRenderer" || component.Selector == nil || component.Selector.Type != protocol.PluginRendererFileExtension {
			t.Fatalf("file selector: %+v", component)
		}
		request := protocol.ReadPluginResourceRequest{OpenPluginResourcesRequest: protocol.OpenPluginResourcesRequest{PluginReleaseRequest: protocol.PluginReleaseRequest{InstallationID: installed.ID, Digest: installed.Selected.Digest}, ComponentID: component.ID}, Path: component.Entry}
		resource := r.must(delivery.PluginsReadResource, request, "").(*protocol.PluginResource)
		body, err := base64.StdEncoding.DecodeString(resource.Body)
		if err != nil {
			t.Fatal(err)
		}
		authored, err := os.ReadFile(filepath.Join(source, component.Entry))
		if err != nil {
			t.Fatal(err)
		}
		if string(body) != string(authored) {
			t.Fatal("file component differs from admitted resource")
		}
	}
}
