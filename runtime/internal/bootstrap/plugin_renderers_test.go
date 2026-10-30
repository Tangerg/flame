package bootstrap

import (
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
)

func TestToolComponentUsesSelectedImmutableBytes(t *testing.T) {
	r := poMCPOpen(t, poMCPConfig(t))
	source, err := filepath.Abs(filepath.Join(firstPartyPackages, "tool-results"))
	if err != nil {
		t.Fatal(err)
	}
	installed := r.must(delivery.PluginsInstall, protocol.InstallPluginRequest{Source: source}, "install-tool-component").(*protocol.PluginInstallation)
	if len(installed.Selected.Diagnostics) != 0 {
		t.Fatalf("admission: %+v", installed.Selected)
	}
	var component protocol.PluginClientComponent
	webBrowse := false
	shaped := false
	for _, value := range installed.Selected.Components {
		if value.Selector != nil && value.Selector.Type == extension.RendererResultShape {
			if value.Selector.Name != "" || !value.Selector.Shape.Valid() {
				t.Fatalf("published result selector: %+v", value.Selector)
			}
			if err := protocol.ValidateWireTree(value); err != nil {
				t.Fatalf("published result component: %v", err)
			}
			shaped = true
		}
		if value.ID == "web-browse" && value.Selector != nil && value.Selector.Type == extension.RendererToolName && value.Selector.Name == "web_browse" {
			webBrowse = true
		}
		if value.ID == "grep" {
			component = value
		}
	}
	if !webBrowse {
		t.Fatal("captured web page continuation has no installed result renderer")
	}
	if !shaped {
		t.Fatal("installed package has no structural result selector")
	}
	if component.Selector == nil || component.Selector.Type != extension.RendererToolName || component.Selector.Name != "grep" || component.Scope != extension.ComponentScopeToolResult || component.Placement != extension.ComponentPlacementToolResultRenderer {
		t.Fatalf("selector: %+v", component)
	}
	request := protocol.ReadPluginResourceRequest{OpenPluginResourcesRequest: protocol.OpenPluginResourcesRequest{PluginReleaseRequest: protocol.PluginReleaseRequest{InstallationID: installed.ID, Digest: installed.Selected.Digest}, ComponentID: component.ID}, Path: component.Entry}
	refused := func(input protocol.ReadPluginResourceRequest, want error) {
		t.Helper()
		result := r.endpoint.Invoke(t.Context(), delivery.PluginsReadResource, input, delivery.Options{RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.Version}})
		if !errors.Is(result.Failure, want) {
			t.Fatalf("read: %v, want %v", result.Failure, want)
		}
	}
	refused(request, protocol.ErrPluginDisabled)
	r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: installed.ID, Enabled: true}, "enable-tool-component")
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
		t.Fatal("component differs from admitted bytes")
	}
	wrong := request
	wrong.ComponentID = "unknown"
	refused(wrong, protocol.ErrPluginNotFound)
	wrong = request
	wrong.Digest = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	refused(wrong, protocol.ErrPluginStale)
	r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: installed.ID, Enabled: false}, "disable-tool-component")
	refused(request, protocol.ErrPluginDisabled)
}

func TestFileComponentUsesSelectedImmutableBytes(t *testing.T) {
	r := poMCPOpen(t, poMCPConfig(t))
	source, err := filepath.Abs(filepath.Join(firstPartyPackages, "file-renderers"))
	if err != nil {
		t.Fatal(err)
	}
	installed := r.must(delivery.PluginsInstall, protocol.InstallPluginRequest{Source: source}, "install-file-component").(*protocol.PluginInstallation)
	if len(installed.Selected.Diagnostics) != 0 || len(installed.Selected.Components) != 3 {
		t.Fatalf("file component admission: %+v", installed.Selected)
	}
	r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: installed.ID, Enabled: true}, "enable-file-component")
	for _, component := range installed.Selected.Components {
		if component.Scope != extension.ComponentScopeFile || component.Placement != extension.ComponentPlacementFileRenderer || component.Selector == nil || component.Selector.Type != extension.RendererFileExtension {
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
