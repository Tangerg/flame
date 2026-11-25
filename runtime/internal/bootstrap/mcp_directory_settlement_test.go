package bootstrap

import (
	"errors"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
)

func TestCommittedMCPWithdrawalDetachesToolsEvenIfGrantCleanupFails(t *testing.T) {
	scenarios := []struct {
		name    string
		trigger string
		method  delivery.Name
		request func(protocol.MCPServerID) any
		removed bool
	}{
		{
			name: "disable",
			trigger: `CREATE TRIGGER withhold_grant_confirmation BEFORE UPDATE OF proposal ON mcp_launch_grants
				BEGIN SELECT RAISE(ABORT, 'withheld grant confirmation'); END`,
			method: "mcp.servers.update",
			request: func(id protocol.MCPServerID) any {
				return protocol.UpdateMCPServerRequest{Server: id, Enabled: new(false)}
			},
		},
		{
			name: "remove",
			trigger: `CREATE TRIGGER withhold_grant_removal BEFORE DELETE ON mcp_launch_grants
				BEGIN SELECT RAISE(ABORT, 'withheld grant removal'); END`,
			method: delivery.MCPServersDelete,
			request: func(id protocol.MCPServerID) any {
				return protocol.MCPServerRequest{Server: id}
			},
			removed: true,
		},
	}
	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			url := poMCPRemote(t, "record")
			cfg := poMCPConfig(t)
			runtime := poMCPOpen(t, cfg)
			installExternalConnector(runtime)
			server := runtime.must(delivery.MCPServersCreate, protocol.MCPServerCandidate{
				Name: "grant-cleanup-failure", Enabled: true,
				Connection:       protocol.MCPConnectionInput{Type: protocol.MCPTransportStreamableHTTP, URL: url},
				HandshakeTimeout: protocol.MCPHandshakeTimeout{Type: protocol.MCPHandshakeUnbounded},
			}, "create-before-withdrawal").(*protocol.MCPServer)
			runtime.poMCPTools(&server.ID, func(tools []protocol.MCPTool) bool { return len(tools) == 1 })
			questionCrashDatabase(t, cfg.stores.DataDirectory, scenario.trigger)

			options := protocol.CallMetadata{
				IdempotencyKey: "withdraw-server", RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.Version},
			}
			result := runtime.endpoint.Invoke(t.Context(), scenario.method, scenario.request(server.ID), options)
			if !errors.Is(result.Failure, protocol.ErrInternalError) {
				t.Fatalf("withdrawal hid the local storage failure: %+v", result)
			}
			catalog := runtime.must(delivery.MCPServersList, struct{}{}, "").(*protocol.Page[protocol.MCPServer])
			found := false
			for _, stored := range catalog.Data {
				if stored.ID == server.ID {
					found = true
					if stored.Status.Type != protocol.MCPServerDisabled {
						t.Fatalf("the connector did not commit the disable: %+v", stored)
					}
				}
			}
			if found == scenario.removed {
				t.Fatalf("stored server present = %t after %s", found, scenario.method)
			}
			tools := runtime.must(delivery.MCPToolsList, protocol.MCPListToolsRequest{Server: &server.ID}, "").(*protocol.Page[protocol.MCPTool])
			if len(tools.Data) != 0 {
				t.Errorf("withdrawn MCP server still advertises %d tools", len(tools.Data))
			}
		})
	}
}
