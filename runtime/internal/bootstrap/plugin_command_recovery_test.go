package bootstrap

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	memorydomain "github.com/Tangerg/flame/plugins/memory/backend/domain"
	memoryrpc "github.com/Tangerg/flame/plugins/memory/backend/rpc"
	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/protocol"
)

func TestMissingOwnerReceiptNeverResubmitsAnUnconfirmedCommand(t *testing.T) {
	source, _ := managedMessageLossPackage(t, "memory", "flame.memory/add.v1", "request")
	cfg := poMCPConfig(t)
	r := poMCPOpen(t, cfg)
	installed := poMCPActivate(r, source)
	server := &protocol.MCPServerID{Origin: protocol.MCPOrigin{Type: protocol.MCPOriginInstallation, InstallationID: installed.ID}, Name: "memory"}
	r.poMCPTools(server, func(tools []protocol.MCPTool) bool { return len(tools) == 1 })
	request := memoryAddRequest(r, memoryrpc.AddInput{Scope: memorydomain.ScopeUser, Content: "must not be silently resubmitted"})
	invoke := func() *delivery.Failure {
		return r.endpoint.Invoke(t.Context(), delivery.PluginsCall, request, delivery.Options{IdempotencyKey: "uncertain", RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.ProtocolVersion}}).Failure
	}
	if failure := invoke(); failure == nil {
		t.Fatal("lost request appeared successful")
	}
	r = reopenCommandRuntime(t, r, &cfg)
	r.poMCPTools(server, func(tools []protocol.MCPTool) bool { return len(tools) == 1 })
	if failure := invoke(); failure == nil || !errors.Is(failure, protocol.ErrInternalError) {
		t.Fatalf("missing receipt was settled or resubmitted: %v", failure)
	}
	page := readMemoryPage(r, memoryrpc.ListInput{Scope: memorydomain.ScopeUser})
	if len(page.Data) != 0 {
		t.Fatalf("receipt lookup executed the original mutation: %+v", page.Data)
	}
	assertUnresolvedInstallationIsHeld(t, r, installed)
}

func reopenCommandRuntime(t *testing.T, r *poMCPRuntime, cfg *Config) *poMCPRuntime {
	t.Helper()
	r.close()
	if err := cfg.Stores.Close(); err != nil {
		t.Fatal(err)
	}
	stores, err := persistence.Open(t.Context(), persistence.Config{DataDirectory: cfg.Stores.DataDirectory})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stores.Close() })
	cfg.Stores = stores
	cfg.ProviderRegistry = stores.Providers
	return poMCPOpen(t, *cfg)
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
	r.poMCPTools(server(first.ID), func(tools []protocol.MCPTool) bool { return len(tools) == 1 })
	request := memoryAddRequest(r, memoryrpc.AddInput{Scope: memorydomain.ScopeUser, Content: "original provider's fact"})
	r.must(delivery.PluginsCall, request, "original-command")
	r.must(delivery.PluginsUninstall, protocol.PluginRequest{InstallationID: first.ID}, "uninstall-observed")
	second := r.must(delivery.PluginsInstall, protocol.InstallPluginRequest{Source: source}, "replacement-install").(*protocol.PluginInstallation)
	r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: second.ID, Enabled: true}, "replacement-enable")
	r.must(delivery.PluginsSelectProvider, protocol.SelectPluginProviderRequest{Role: "memory", InstallationID: second.ID, Contribution: "memory-provider"}, "replacement-provider")
	r = reopenCommandRuntime(t, r, &cfg)
	r.poMCPTools(server(second.ID), func(tools []protocol.MCPTool) bool { return len(tools) == 1 })
	replay := r.endpoint.Invoke(t.Context(), delivery.PluginsCall, request, delivery.Options{IdempotencyKey: "original-command", RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.ProtocolVersion}})
	if replay.Failure == nil || !errors.Is(replay.Failure, protocol.ErrInternalError) {
		t.Fatalf("old identity adopted replacement provider: %v", replay.Failure)
	}
	page := readMemoryPage(r, memoryrpc.ListInput{Scope: memorydomain.ScopeUser})
	if len(page.Data) != 0 {
		t.Fatalf("old command mutated replacement owner: %+v", page.Data)
	}
	created := mustMemoryAdd(r, memoryAddRequest(r, memoryrpc.AddInput{Scope: memorydomain.ScopeUser, Content: "original provider's fact"}), "new-command")
	page = readMemoryPage(r, memoryrpc.ListInput{Scope: memorydomain.ScopeUser})
	if len(page.Data) != 1 || !reflect.DeepEqual(page.Data[0], *created) {
		t.Fatalf("new command did not use replacement owner: %+v", page.Data)
	}
}
