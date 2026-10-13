package bootstrap

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/protocol"
)

func TestProviderChoiceDoesNotFollowEnablementAndNeverRetargetsOriginalCommand(t *testing.T) {
	loss, evidence := managedMessageLossPackage(t, "memory", "flame.memory/add.v1", "response")
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
	r.poMCPTools(server(first), func(tools []protocol.MCPTool) bool { return len(tools) == 1 })
	r.poMCPTools(server(second), func(tools []protocol.MCPTool) bool { return len(tools) == 1 })
	list := func() delivery.Result {
		return r.endpoint.Invoke(t.Context(), delivery.AgentMemoryList, protocol.AgentMemoryListRequest{Scope: protocol.AgentMemoryScopeUser}, delivery.Options{RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.ProtocolVersion}})
	}
	if response := list(); response.Failure == nil || !errors.Is(response.Failure, protocol.ErrPluginUnavailable) {
		t.Fatalf("enablement chose a provider: %v", response.Failure)
	}
	for _, invalid := range []protocol.SelectPluginProviderRequest{
		{Role: "schedules", InstallationID: first.ID, Contribution: "memory-provider"},
		{Role: "memory", InstallationID: first.ID, Contribution: "another"},
	} {
		response := r.endpoint.Invoke(t.Context(), delivery.PluginsSelectProvider, invalid, delivery.Options{RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.ProtocolVersion}})
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
	request := protocol.AgentMemoryAddRequest{Scope: protocol.AgentMemoryScopeUser, Content: "original owner's fact"}
	if result := r.endpoint.Invoke(t.Context(), delivery.AgentMemoryAdd, request, delivery.Options{IdempotencyKey: "uncertain", RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.ProtocolVersion}}); result.Failure == nil {
		t.Fatal("lost response became confirmed")
	}
	if _, err := os.Stat(evidence); err != nil {
		t.Fatalf("no committed response witnessed: %v", err)
	}
	choose(second, "choose-second")
	assertUnresolvedInstallationIsHeld(t, r, first)
	r = reopenCommandRuntime(t, r, &cfg)
	assertUnresolvedInstallationIsHeld(t, r, first)
	r.poMCPTools(server(first), func(tools []protocol.MCPTool) bool { return len(tools) == 1 })
	r.must(delivery.AgentMemoryAdd, request, "uncertain")
	page := r.must(delivery.AgentMemoryList, protocol.AgentMemoryListRequest{Scope: protocol.AgentMemoryScopeUser}, "").(*protocol.Page[protocol.AgentMemoryItem])
	if len(page.Data) != 0 {
		t.Fatalf("original recovery mutated selected replacement: %+v", page.Data)
	}
	r.must(delivery.AgentMemoryAdd, protocol.AgentMemoryAddRequest{Scope: protocol.AgentMemoryScopeUser, Content: "selected owner's fact"}, "new-command")
	r = reopenCommandRuntime(t, r, &cfg)
	r.poMCPTools(server(second), func(tools []protocol.MCPTool) bool { return len(tools) == 1 })
	page = r.must(delivery.AgentMemoryList, protocol.AgentMemoryListRequest{Scope: protocol.AgentMemoryScopeUser}, "").(*protocol.Page[protocol.AgentMemoryItem])
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
