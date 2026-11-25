package bootstrap

import (
	"slices"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
)

func TestMCPToolExposureAndItsReceiptCommitTogether(t *testing.T) {
	const setExposure delivery.Name = "mcp.tools.setExposure"
	const toolName = "record"
	remote := poMCPRemote(t, toolName)
	cfg := poMCPConfig(t)
	host := poMCPOpen(t, cfg)
	installExternalConnector(host)
	serverID := host.createUserServer(protocol.MCPServerCandidate{
		Name: "exposure-commit", Enabled: true,
		Connection:       protocol.MCPConnectionInput{Type: protocol.MCPTransportStreamableHTTP, URL: remote},
		HandshakeTimeout: protocol.MCPHandshakeTimeout{Type: protocol.MCPHandshakeUnbounded},
	}, "create-exposure-server")
	host.poMCPTools(&serverID, func(tools []protocol.MCPTool) bool {
		return len(tools) == 1 && tools[0].Name == toolName
	})

	exposure := func(want ...string) {
		t.Helper()
		result, err := host.api.GetMCPToolExposure(t.Context(), protocol.MCPServerRequest{Server: serverID})
		if err != nil || result == nil || !slices.Equal(result.DisabledTools, want) {
			t.Fatalf("disabled tools = %+v, %v, want %v", result, err, want)
		}
	}
	disable := protocol.SetMCPToolExposureRequest{Server: serverID, Name: toolName, Disabled: true}
	const disableKey = "disable-original-tool"
	allowReceipt := refuseCommandReceipt(t, cfg.stores.DataDirectory)
	refused := host.endpoint.Invoke(t.Context(), setExposure, disable, protocol.CallMetadata{IdempotencyKey: disableKey})
	allowReceipt()
	if refused.Failure == nil {
		t.Fatal("disabled the tool despite a refused receipt")
	}
	exposure()
	host.must(setExposure, disable, disableKey)
	exposure(toolName)

	host.must(setExposure, protocol.SetMCPToolExposureRequest{Server: serverID, Name: toolName}, "enable-tool")
	host.must(setExposure, disable, disableKey)
	exposure()
}
