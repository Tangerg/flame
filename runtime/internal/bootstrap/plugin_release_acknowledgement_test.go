package bootstrap

import (
	"errors"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
)

func releaseChoiceSource(t *testing.T, contents string) string {
	t.Helper()
	source := poMCPPackage(t, map[string]string{"plugin.json": poMCPManifest, "notes.txt": contents})
	authorPluginPackage(t, source)
	return source
}

func TestPluginStagingCommitsWithItsOriginalAcknowledgement(t *testing.T) {
	cfg := poMCPConfig(t)
	r := poMCPOpen(t, cfg)
	installed := r.installAndInspect(releaseChoiceSource(t, "original"), "install-for-staging")
	request := protocol.StagePluginRequest{InstallationID: installed.ID, Source: releaseChoiceSource(t, "first candidate")}
	const key = "stage-original-candidate"
	allowReceipt := refuseCommandReceipt(t, cfg.stores.DataDirectory)
	failed := r.endpoint.Invoke(t.Context(), delivery.PluginsStage, request, protocol.CallMetadata{
		IdempotencyKey: key, RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.Version},
	})
	allowReceipt()
	if failed.Failure == nil {
		t.Fatal("staging succeeded with a refused receipt")
	}

	r = reopenPluginRuntime(t, r, &cfg)
	if current := r.pluginInstallation(installed.ID); current.Staged != nil || current.Selected.Digest != installed.Selected.Digest {
		t.Fatal("a candidate escaped the staging acknowledgement transaction")
	}
	first := r.must(delivery.PluginsStage, request, key).(*protocol.PluginReleaseRef)
	replacement := protocol.StagePluginRequest{InstallationID: installed.ID, Source: releaseChoiceSource(t, "second candidate")}
	second := r.must(delivery.PluginsStage, replacement, "stage-new-candidate").(*protocol.PluginReleaseRef)
	replayed := r.must(delivery.PluginsStage, request, key).(*protocol.PluginReleaseRef)
	if first.InstallationID != installed.ID || second.InstallationID != installed.ID || first.Digest == second.Digest || *replayed != *first {
		t.Fatal("staging lost an exact candidate identity")
	}
	if current := r.pluginInstallation(installed.ID); current.Staged == nil || current.Staged.Digest != second.Digest || current.Selected.Digest != installed.Selected.Digest {
		t.Fatal("staging replay displaced the later candidate")
	}
}

func TestPluginSelectionCommitsWithItsOriginalAcknowledgement(t *testing.T) {
	cfg := poMCPConfig(t)
	r := poMCPOpen(t, cfg)
	installed := r.installAndInspect(releaseChoiceSource(t, "original"), "install-for-selection")
	stage := func(contents, key string) protocol.PluginReleaseRef {
		t.Helper()
		return *r.must(delivery.PluginsStage, protocol.StagePluginRequest{InstallationID: installed.ID, Source: releaseChoiceSource(t, contents)}, key).(*protocol.PluginReleaseRef)
	}
	request := stage("approved candidate", "stage-for-selection")
	const key = "select-approved-candidate"
	allowReceipt := refuseCommandReceipt(t, cfg.stores.DataDirectory)
	failed := r.endpoint.Invoke(t.Context(), delivery.PluginsSelect, request, protocol.CallMetadata{
		IdempotencyKey: key, RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.Version},
	})
	allowReceipt()
	if failed.Failure == nil {
		t.Fatal("selection succeeded with a refused receipt")
	}

	r = reopenPluginRuntime(t, r, &cfg)
	if current := r.pluginInstallation(installed.ID); current.Selected.Digest != installed.Selected.Digest || current.Staged == nil || current.Staged.Digest != request.Digest {
		t.Fatal("the selected release and staged slot changed after a refused receipt")
	}
	first := r.must(delivery.PluginsSelect, request, key).(*protocol.PluginReleaseRef)
	second := stage("later candidate", "stage-after-selection")
	r.must(delivery.PluginsSelect, second, "select-later-candidate")
	replayed := r.must(delivery.PluginsSelect, request, key).(*protocol.PluginReleaseRef)
	if *first != request || *replayed != request {
		t.Fatal("selection did not retain its approved digest")
	}
	if current := r.pluginInstallation(installed.ID); current.Selected.Digest != second.Digest || current.Staged != nil {
		t.Fatal("selection replay changed the later release")
	}
	stale := r.endpoint.Invoke(t.Context(), delivery.PluginsSelect, request, protocol.CallMetadata{
		IdempotencyKey: "new-selection-of-old-release", RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.Version},
	})
	if stale.Failure == nil || !errors.Is(stale.Failure, protocol.ErrPluginStale) {
		t.Fatalf("selection of a consumed candidate returned %v", stale.Failure)
	}
}
