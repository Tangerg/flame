package bootstrap

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
)

func TestPluginUninstallCommitsWithItsOriginalAcknowledgement(t *testing.T) {
	cfg := poMCPConfig(t)
	r := poMCPOpen(t, cfg)
	source := filepath.Join(firstPartyPackages, "guardrails")
	installed := poMCPActivate(r, source)
	request := protocol.PluginRequest{InstallationID: installed.ID}
	const key = "uninstall-original-installation"

	allowReceipt := refuseCommandReceipt(t, cfg.stores.DataDirectory)
	failed := r.endpoint.Invoke(t.Context(), delivery.PluginsUninstall, request, protocol.CallMetadata{
		IdempotencyKey: key, RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.Version},
	})
	allowReceipt()
	if failed.Failure == nil {
		t.Fatal("uninstall succeeded with a refused receipt")
	}

	r = reopenPluginRuntime(t, r, &cfg)
	if previous := r.pluginInstallation(installed.ID); !previous.Enabled || previous.Selected.Digest != installed.Selected.Digest {
		t.Fatal("a refused uninstall changed the original installation")
	}
	r.must(delivery.PluginsUninstall, request, key)
	page := r.must(delivery.PluginsList, struct{}{}, "").(*protocol.Page[protocol.PluginInstallation])
	if len(page.Data) != 0 {
		t.Fatal("an acknowledged uninstall left the original installation")
	}
	missing := r.endpoint.Invoke(t.Context(), delivery.PluginsUninstall, request, protocol.CallMetadata{
		IdempotencyKey: "uninstall-absent-installation", RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.Version},
	})
	if !errors.Is(missing.Failure, protocol.ErrPluginNotFound) {
		t.Fatalf("uninstalling an absent installation returned %v", missing.Failure)
	}

	replacement := r.installAndInspect(source, "install-after-uninstall")
	if replacement.ID == installed.ID {
		t.Fatal("a new installation reused the removed installation's identity")
	}
	r.must(delivery.PluginsUninstall, request, key)
	if current := r.pluginInstallation(replacement.ID); current.Selected.Digest != installed.Selected.Digest {
		t.Fatal("the repeated uninstall changed the replacement installation")
	}
}
