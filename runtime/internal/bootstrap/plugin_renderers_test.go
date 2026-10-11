package bootstrap

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/protocol"
)

func TestOptionalRendererUsesApprovedImmutableBytes(t *testing.T) {
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
	refused(request, protocol.ErrPluginUnapproved)
	r.must(delivery.PluginsApprove, request.PluginReleaseRequest, "approve-renderer")
	refused(request, protocol.ErrPluginUnapproved)
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
	r.must(delivery.PluginsRevoke, protocol.PluginRequest{InstallationID: installed.ID}, "revoke-renderer")
	refused(request, protocol.ErrPluginUnapproved)
}
