package bootstrap

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	memorycontract "github.com/Tangerg/flame/packages/go-sdk/plugin/memory"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
)

func TestProviderChoiceDoesNotFollowEnablementAndNeverRetargetsOriginalCommand(t *testing.T) {
	loss, evidence := managedMessageLossPackage(t, "memory", "flame.memory/add.v2", "response")
	cfg := poMCPConfig(t)
	r := poMCPOpen(t, cfg)
	install := func(source, key string) *protocol.PluginInstallation {
		value := r.must(delivery.PluginsInstall, protocol.InstallPluginRequest{Source: source}, key+"-install").(*protocol.PluginInstallation)
		return r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: value.ID, Enabled: true}, key+"-enable").(*protocol.PluginInstallation)
	}
	first := install(loss, "first")
	second := install(filepath.Join(firstPartyPackages, "memory"), "second")
	server := func(value *protocol.PluginInstallation) *protocol.MCPServerID {
		return &protocol.MCPServerID{Origin: protocol.MCPOrigin{Type: protocol.MCPOriginInstallation, InstallationID: value.ID}, Name: "memory"}
	}
	r.poMCPTools(server(first), memoryToolsReady)
	r.poMCPTools(server(second), memoryToolsReady)
	list := func() delivery.Result {
		return memoryQueryResult(r, memorycontract.ListV2Input{Scope: "user"})
	}
	if response := list(); response.Failure == nil || !errors.Is(response.Failure, protocol.ErrPluginUnavailable) {
		t.Fatalf("enablement chose a provider: %v", response.Failure)
	}
	for _, invalid := range []protocol.SelectPluginProviderRequest{
		{Role: "schedules", InstallationID: first.ID, Contribution: "memory-provider"},
		{Role: "memory", InstallationID: first.ID, Contribution: "another"},
	} {
		response := r.endpoint.Invoke(t.Context(), delivery.PluginsSelectProvider, invalid, delivery.Options{RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.Version}})
		if response.Failure == nil || !errors.Is(response.Failure, protocol.ErrInvalidParams) {
			t.Fatalf("invalid role/contribution accepted: %+v, %v", invalid, response.Failure)
		}
	}
	choices := r.must(delivery.PluginsListProviders, struct{}{}, "").(*protocol.Page[protocol.PluginProviderSelection])
	if len(choices.Data) != 0 {
		t.Fatalf("refused choice changed configuration: %+v", choices.Data)
	}
	choose := func(value *protocol.PluginInstallation, key string) {
		r.must(delivery.PluginsSelectProvider, protocol.SelectPluginProviderRequest{Role: "memory", InstallationID: value.ID, Contribution: "memory-provider"}, key)
	}
	choose(first, "choose-first")
	request := memoryAddRequest(r, memorycontract.AddV2Input{Scope: "user", Content: "original owner's fact"})
	if result := r.endpoint.Invoke(t.Context(), delivery.PluginsCall, request, delivery.Options{IdempotencyKey: "uncertain", RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.Version}}); result.Failure == nil {
		t.Fatal("lost response became confirmed")
	}
	if _, err := os.Stat(evidence); err != nil {
		t.Fatalf("no committed response witnessed: %v", err)
	}
	choose(second, "choose-second")
	assertUnresolvedInstallationIsHeld(t, r, first)
	r = reopenCommandRuntime(t, r, &cfg)
	assertUnresolvedInstallationIsHeld(t, r, first)
	r.poMCPTools(server(first), memoryToolsReady)
	r.must(delivery.PluginsCall, request, "uncertain")
	page := readMemoryPage(r, memorycontract.ListV2Input{Scope: "user"})
	if len(page.Data) != 0 {
		t.Fatalf("original recovery mutated selected replacement: %+v", page.Data)
	}
	r.must(delivery.PluginsCall, memoryAddRequest(r, memorycontract.AddV2Input{Scope: "user", Content: "selected owner's fact"}), "new-command")
	r = reopenCommandRuntime(t, r, &cfg)
	r.poMCPTools(server(second), memoryToolsReady)
	page = readMemoryPage(r, memorycontract.ListV2Input{Scope: "user"})
	if len(page.Data) != 1 || page.Data[0].Content != "selected owner's fact" {
		t.Fatalf("restart changed provider choice or private data: %+v", page.Data)
	}
	r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: second.ID, Enabled: false}, "disable-selected")
	if response := list(); response.Failure == nil || !errors.Is(response.Failure, protocol.ErrPluginUnavailable) {
		t.Fatalf("disabled selection chose another enabled provider: %v", response.Failure)
	}
	choices = r.must(delivery.PluginsListProviders, struct{}{}, "").(*protocol.Page[protocol.PluginProviderSelection])
	if len(choices.Data) != 1 || choices.Data[0].InstallationID != second.ID {
		t.Fatalf("availability changed composition: %+v", choices.Data)
	}
}
