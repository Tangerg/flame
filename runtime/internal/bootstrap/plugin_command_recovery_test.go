package bootstrap

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	memorycontract "github.com/Tangerg/flame/packages/go-sdk/plugin/memory"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	"github.com/Tangerg/flame/runtime/internal/delivery"
)

func TestMissingOwnerReceiptNeverResubmitsAnUnconfirmedCommand(t *testing.T) {
	source, _ := managedMessageLossPackage(t, "memory", "flame.memory/add.v2", "request")
	cfg := poMCPConfig(t)
	r := poMCPOpen(t, cfg)
	installed := poMCPActivate(r, source)
	server := &protocol.MCPServerID{Origin: protocol.MCPOrigin{Type: protocol.MCPOriginInstallation, InstallationID: installed.ID}, Name: "memory"}
	r.poMCPTools(server, memoryToolsReady)
	request := memoryAddRequest(r, memorycontract.AddV2Input{Scope: memorycontract.AddV2InputScopeUser, Content: "must not be silently resubmitted"})
	invoke := func() *protocol.Failure {
		return r.endpoint.Invoke(t.Context(), delivery.PluginsCall, request, delivery.Options{IdempotencyKey: "uncertain", RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.Version}}).Failure
	}
	if failure := invoke(); failure == nil {
		t.Fatal("lost request appeared successful")
	}
	r = reopenPluginRuntime(t, r, &cfg)
	r.poMCPTools(server, memoryToolsReady)
	if failure := invoke(); failure == nil || !errors.Is(failure, protocol.ErrInternalError) {
		t.Fatalf("missing receipt was settled or resubmitted: %v", failure)
	}
	page := readMemoryPage(r, memorycontract.ListV2Input{Scope: memorycontract.ListV2InputScopeUser})
	if len(page.Data) != 0 {
		t.Fatalf("receipt lookup executed the original mutation: %+v", page.Data)
	}
	assertUnresolvedInstallationIsHeld(t, r, installed)
}

func reopenPluginRuntime(t *testing.T, r *poMCPRuntime, cfg *assemblyConfig) *poMCPRuntime {
	t.Helper()
	r.close()
	reopenPluginStores(t, cfg)
	return poMCPOpen(t, *cfg)
}

func reopenPluginStores(t *testing.T, cfg *assemblyConfig) {
	t.Helper()
	directory := cfg.stores.DataDirectory
	if err := cfg.stores.Close(); err != nil {
		t.Fatal(err)
	}
	stores, err := persistence.Open(t.Context(), persistence.Config{DataDirectory: directory})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stores.Close() })
	cfg.stores = stores
	cfg.providerRegistry = stores.Providers
}

func stageCommandReplacement(t *testing.T, r *poMCPRuntime, installed *protocol.PluginInstallation, source string) *protocol.PluginInstallation {
	t.Helper()
	candidate := filepath.Join(t.TempDir(), "candidate")
	if err := os.CopyFS(candidate, os.DirFS(source)); err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(candidate, "plugin.json")
	data, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifest, append(data, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	authorPluginPackage(t, candidate)
	staged := r.must(delivery.PluginsStage, protocol.StagePluginRequest{InstallationID: installed.ID, Source: candidate}, "stage-candidate").(*protocol.PluginInstallation)
	if staged.Staged == nil || staged.Staged.Digest == installed.Selected.Digest {
		t.Fatal("fixture did not stage distinct bytes")
	}
	return staged
}

func TestObservedCommandNeverRetargetsToReplacementProvider(t *testing.T) {
	cfg := poMCPConfig(t)
	r := poMCPOpen(t, cfg)
	source := filepath.Join(firstPartyPackages, "memory")
	first := poMCPActivate(r, source)
	server := func(id string) *protocol.MCPServerID {
		return &protocol.MCPServerID{Origin: protocol.MCPOrigin{Type: protocol.MCPOriginInstallation, InstallationID: id}, Name: "memory"}
	}
	r.poMCPTools(server(first.ID), memoryToolsReady)
	request := memoryAddRequest(r, memorycontract.AddV2Input{Scope: memorycontract.AddV2InputScopeUser, Content: "original provider's fact"})
	r.must(delivery.PluginsCall, request, "original-command")
	r.must(delivery.PluginsUninstall, protocol.PluginRequest{InstallationID: first.ID}, "uninstall-observed")
	second := r.must(delivery.PluginsInstall, protocol.InstallPluginRequest{Source: source}, "replacement-install").(*protocol.PluginInstallation)
	r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: second.ID, Enabled: true}, "replacement-enable")
	r.must(delivery.PluginsSelectProvider, protocol.SelectPluginProviderRequest{Role: "memory", InstallationID: second.ID, Contribution: "memory-provider"}, "replacement-provider")
	r = reopenPluginRuntime(t, r, &cfg)
	r.poMCPTools(server(second.ID), memoryToolsReady)
	replay := r.endpoint.Invoke(t.Context(), delivery.PluginsCall, request, delivery.Options{IdempotencyKey: "original-command", RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.Version}})
	if replay.Failure == nil || !errors.Is(replay.Failure, protocol.ErrInternalError) {
		t.Fatalf("old identity adopted replacement provider: %v", replay.Failure)
	}
	page := readMemoryPage(r, memorycontract.ListV2Input{Scope: memorycontract.ListV2InputScopeUser})
	if len(page.Data) != 0 {
		t.Fatalf("old command mutated replacement owner: %+v", page.Data)
	}
	created := mustMemoryAdd(r, memoryAddRequest(r, memorycontract.AddV2Input{Scope: memorycontract.AddV2InputScopeUser, Content: "original provider's fact"}), "new-command")
	page = readMemoryPage(r, memorycontract.ListV2Input{Scope: memorycontract.ListV2InputScopeUser})
	if len(page.Data) != 1 || !reflect.DeepEqual(page.Data[0], *created) {
		t.Fatalf("new command did not use replacement owner: %+v", page.Data)
	}
}
