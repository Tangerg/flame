package bootstrap

import (
	json "encoding/json/v2"
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/qimu/portable"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func TestA2AInstalledConnectorOwnsConfigurationAndAvailability(t *testing.T) {
	t.Setenv("A2A_AGENTS", "invalid parent configuration")
	r := poMCPOpen(t, poMCPConfig(t))
	installed := poMCPActivate(r, filepath.Join(firstPartyPackages, "a2a"))
	server := &protocol.MCPServerID{Origin: protocol.MCPOrigin{Type: protocol.MCPOriginInstallation, InstallationID: installed.ID}, Name: "a2a"}
	r.poMCPTools(server, func(tools []protocol.MCPTool) bool { return len(tools) == 4 })
	contract, err := portable.ParseCapability([]byte(testsupport.A2ACapabilityJSON))
	if err != nil {
		t.Fatal(err)
	}
	call := protocol.CallPluginRequest{
		InstallationID: installed.ID, Digest: installed.Selected.Digest,
		Contribution: "remote-agents", Contract: contract.Identity().String(),
		Target: portable.Target{Scope: extension.ScopeInstallation, Reference: installed.ID},
		Method: testsupport.A2AAgentsV1Method, Input: `{}`,
	}
	result := r.must(delivery.PluginsCall, call, "").(*protocol.CallPluginResponse)
	var agents struct {
		Agents []struct{ Name string } `json:"agents"`
	}
	if result.Failure != nil || json.Unmarshal([]byte(result.Output), &agents) != nil || len(agents.Agents) != 0 {
		t.Fatalf("parent configuration became installation authority: %+v", result)
	}
	call.Method, call.Input = testsupport.A2ASendV1Method, `{"agent":"missing","message":"hello"}`
	result = r.must(delivery.PluginsCall, call, "send-unconfigured").(*protocol.CallPluginResponse)
	if result.Failure == nil || result.Failure.Code != "agent_unconfigured" {
		t.Fatalf("unconfigured agent was not refused: %+v", result)
	}
	r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: installed.ID, Enabled: false}, "disable-a2a")
	r.poMCPTools(server, func(tools []protocol.MCPTool) bool { return len(tools) == 0 })
	call.Method, call.Input = testsupport.A2AAgentsV1Method, `{}`
	response := r.endpoint.Invoke(t.Context(), delivery.PluginsCall, call, delivery.Options{RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.Version}})
	if response.Failure == nil {
		t.Fatal("disabled connector accepted a new call")
	}
}
