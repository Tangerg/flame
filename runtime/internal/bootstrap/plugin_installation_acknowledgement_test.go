package bootstrap

import (
	"slices"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
)

func TestPluginInstallCommitsWithItsOriginalAcknowledgement(t *testing.T) {
	cfg := poMCPConfig(t)
	r := poMCPOpen(t, cfg)
	request := protocol.InstallPluginRequest{Source: releaseChoiceSource(t, "installation receipt")}
	const key = "install-original-release"

	allowReceipt := refuseCommandReceipt(t, cfg.stores.DataDirectory)
	failed := r.endpoint.Invoke(t.Context(), delivery.PluginsInstall, request, protocol.CallMetadata{
		IdempotencyKey: key, RequestMeta: protocol.RequestMeta{ProtocolVersion: new(protocol.Version)},
	})
	allowReceipt()
	if failed.Failure == nil {
		t.Fatal("installation succeeded with a refused receipt")
	}

	r = reopenPluginRuntime(t, r, &cfg)
	if page := r.must(delivery.PluginsList, struct{}{}, "").(*protocol.Page[protocol.PluginInstallation]); len(page.Data) != 0 {
		t.Fatalf("%d installations escaped the acknowledgement transaction", len(page.Data))
	}
	first := r.must(delivery.PluginsInstall, request, key).(*protocol.PluginReleaseRef)
	r.must(delivery.PluginsUninstall, protocol.PluginRequest{InstallationID: first.InstallationID}, "uninstall-acknowledged-installation")
	replayed := r.must(delivery.PluginsInstall, request, key).(*protocol.PluginReleaseRef)
	if *replayed != *first {
		t.Fatal("install replay changed the original release identity")
	}
	if page := r.must(delivery.PluginsList, struct{}{}, "").(*protocol.Page[protocol.PluginInstallation]); len(page.Data) != 0 {
		t.Fatal("install replay restored the removed installation")
	}
}

func TestPluginConfigurationCommitsWithItsOriginalAcknowledgement(t *testing.T) {
	cfg := poMCPConfig(t)
	r := poMCPOpen(t, cfg)
	const server = "reviews"
	source := poMCPPackage(t, map[string]string{
		"plugin.json": poMCPManifest,
		"mcp.json":    `{"$schema":"https://agent-plugins.org/schemas/1.0.0/mcp.schema.json","mcpServers":{"reviews":` + testMCPServerJSON(t, poMCPRemote(t, "record")) + `}}`,
	})
	authorPluginPackage(t, source)
	installed := r.installAndInspect(source, "install-for-configuration")
	disable := protocol.ConfigurePluginRequest{
		InstallationID: installed.ID, Digest: installed.Selected.Digest,
		ServerChanges: map[string]protocol.PluginComponentChange{server: protocol.PluginComponentDisable},
	}
	const key = "configure-original-disable"
	allowReceipt := refuseCommandReceipt(t, cfg.stores.DataDirectory)
	failed := r.endpoint.Invoke(t.Context(), delivery.PluginsConfigure, disable, protocol.CallMetadata{
		IdempotencyKey: key, RequestMeta: protocol.RequestMeta{ProtocolVersion: new(protocol.Version)},
	})
	allowReceipt()
	if failed.Failure == nil {
		t.Fatal("configuration succeeded with a refused receipt")
	}

	r = reopenPluginRuntime(t, r, &cfg)
	requireDisabled := func(want bool) {
		t.Helper()
		if current := r.pluginInstallation(installed.ID); slices.Contains(current.DisabledServers, server) != want {
			t.Fatalf("disabled server state = %v, want %t", current.DisabledServers, want)
		}
	}
	requireDisabled(false)
	r.must(delivery.PluginsConfigure, disable, key)
	requireDisabled(true)
	enable := protocol.ConfigurePluginRequest{
		InstallationID: installed.ID, Digest: installed.Selected.Digest,
		ServerChanges: map[string]protocol.PluginComponentChange{server: protocol.PluginComponentEnable},
	}
	r.must(delivery.PluginsConfigure, enable, "configure-later-enable")
	r.must(delivery.PluginsConfigure, disable, key)
	requireDisabled(false)

	const unchanged = "confirm-current-server-configuration"
	r.must(delivery.PluginsConfigure, enable, unchanged)
	r.must(delivery.PluginsConfigure, disable, "configure-later-disable")
	r.must(delivery.PluginsConfigure, enable, unchanged)
	requireDisabled(true)
}
