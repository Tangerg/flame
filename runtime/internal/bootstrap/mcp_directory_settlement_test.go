package bootstrap

import (
	"errors"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
)

func TestCommittedMCPWithdrawalDetachesToolsEvenIfLocalSettlementFails(t *testing.T) {
	scenarios := []struct {
		name        string
		triggerName string
		trigger     string
		method      delivery.Name
		request     func(protocol.MCPServerID) any
		removed     bool
	}{
		{
			name: "disable", triggerName: "withhold_directory_receipt",
			trigger: `CREATE TRIGGER withhold_directory_receipt BEFORE UPDATE OF payload ON idempotency_records
				WHEN OLD.key = 'withdraw-server'
				BEGIN SELECT RAISE(ABORT, 'withheld directory receipt'); END`,
			method: delivery.MCPServersUpdate,
			request: func(id protocol.MCPServerID) any {
				return protocol.UpdateMCPServerRequest{Server: id, Enabled: new(false)}
			},
		},
		{
			name: "remove", triggerName: "withhold_grant_removal",
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
			serverID := runtime.createUserServer(protocol.MCPServerCandidate{
				Name: "grant-cleanup-failure", Enabled: true,
				Connection:       protocol.MCPConnectionInput{Type: protocol.MCPTransportStreamableHTTP, URL: url},
				HandshakeTimeout: protocol.MCPHandshakeTimeout{Type: protocol.MCPHandshakeUnbounded},
			}, "create-before-withdrawal")
			runtime.poMCPTools(&serverID, func(tools []protocol.MCPTool) bool { return len(tools) == 1 })
			questionCrashDatabase(t, cfg.stores.DataDirectory, scenario.trigger)

			options := protocol.CallMetadata{
				IdempotencyKey: "withdraw-server", RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.Version},
			}
			result := runtime.endpoint.Invoke(t.Context(), scenario.method, scenario.request(serverID), options)
			if !errors.Is(result.Failure, protocol.ErrIdempotencyInProgress) {
				t.Fatalf("withdrawal hid the local storage failure: %+v", result)
			}
			catalog := runtime.must(delivery.MCPServersList, struct{}{}, "").(*protocol.Page[protocol.MCPServer])
			found := false
			for _, stored := range catalog.Data {
				if stored.ID == serverID {
					found = true
					if stored.Status.Type != protocol.MCPServerDisabled {
						t.Fatalf("the connector did not commit the disable: %+v", stored)
					}
				}
			}
			if found == scenario.removed {
				t.Fatalf("stored server present = %t after %s", found, scenario.method)
			}
			tools := runtime.must(delivery.MCPToolsList, protocol.MCPListToolsRequest{Server: &serverID}, "").(*protocol.Page[protocol.MCPTool])
			if len(tools.Data) != 0 {
				t.Errorf("withdrawn MCP server still advertises %d tools", len(tools.Data))
			}

			questionCrashDatabase(t, cfg.stores.DataDirectory, "DROP TRIGGER "+scenario.triggerName)
			result = runtime.endpoint.Invoke(t.Context(), scenario.method, scenario.request(serverID), options)
			if result.Failure != nil || result.Value != (struct{}{}) {
				t.Fatalf("original withdrawal after settlement resumed = %+v, %v", result.Value, result.Failure)
			}
		})
	}
}

func TestMCPReplacementLaunchUsesTheConnectorReceipt(t *testing.T) {
	original := poMCPRemote(t, "original")
	replacement := poMCPRemote(t, "replacement")
	cfg := poMCPConfig(t)
	host := poMCPOpen(t, cfg)
	installExternalConnector(host)
	id := host.createUserServer(protocol.MCPServerCandidate{
		Name: "replace-transport", Enabled: true,
		Connection:       protocol.MCPConnectionInput{Type: protocol.MCPTransportStreamableHTTP, URL: original},
		HandshakeTimeout: protocol.MCPHandshakeTimeout{Type: protocol.MCPHandshakeUnbounded},
	}, "create-original-transport")
	expectTool := func(name string) {
		t.Helper()
		host.poMCPTools(&id, func(tools []protocol.MCPTool) bool {
			return len(tools) == 1 && tools[0].Name == name
		})
	}
	expectTool("original")

	allowReceipt := refuseCommandReceipt(t, cfg.stores.DataDirectory)
	change := protocol.UpdateMCPServerRequest{
		Server: id, Connection: &protocol.MCPConnectionInput{Type: protocol.MCPTransportStreamableHTTP, URL: replacement},
	}
	const key = "replace-original-transport"
	result := host.endpoint.Invoke(t.Context(), delivery.MCPServersUpdate, change, protocol.CallMetadata{IdempotencyKey: key})
	if !errors.Is(result.Failure, protocol.ErrIdempotencyInProgress) {
		t.Fatalf("the unconfirmed public acknowledgement = %+v", result)
	}
	expectTool("replacement")

	allowReceipt()
	host.must(delivery.MCPServersUpdate, change, key)
	expectTool("replacement")
}
