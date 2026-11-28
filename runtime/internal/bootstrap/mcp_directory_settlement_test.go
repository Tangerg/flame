package bootstrap

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
)

func TestMCPExternalWritesRequireCallerIdentity(t *testing.T) {
	url := poMCPRemote(t, "record")
	cfg := poMCPConfig(t)
	host := poMCPOpen(t, cfg)
	connector := installExternalConnector(host)
	candidate := protocol.MCPServerCandidate{
		Name: "managed-transport", Enabled: true,
		Connection:       protocol.MCPConnectionInput{Type: protocol.MCPTransportStreamableHTTP, URL: url},
		HandshakeTimeout: protocol.MCPHandshakeTimeout{Type: protocol.MCPHandshakeUnbounded},
	}
	server := host.createUserServer(candidate, "create-managed-transport")
	host.poMCPTools(&server, func(tools []protocol.MCPTool) bool { return len(tools) == 1 })
	withoutKey := func(method delivery.Name, request any) {
		t.Helper()
		result := host.endpoint.Invoke(t.Context(), method, request, protocol.CallMetadata{})
		if result.Failure == nil || !errors.Is(result.Failure, protocol.ErrInvalidParams) {
			t.Errorf("%s without a key = %+v, %v; want invalid params", method, result.Value, result.Failure)
		}
	}
	exists := func(name string) bool {
		t.Helper()
		page := host.must(delivery.MCPServersList, struct{}{}, "").(*protocol.Page[protocol.MCPServer])
		for _, entry := range page.Data {
			if entry.ID.Origin.Type == protocol.MCPOriginUser && entry.ID.Name == name {
				return true
			}
		}
		return false
	}

	candidate.Name = "unkeyed-server"
	withoutKey(delivery.MCPServersCreate, candidate)
	if exists(candidate.Name) {
		t.Error("creation without a key changed the connector directory")
	}
	target := host.userServerTarget(server)
	withoutKey(delivery.MCPServersUpdate, protocol.UpdateMCPServerRequest{
		MCPDirectoryTarget: target, Description: new("unkeyed description"),
	})
	if updated := host.userServerTarget(server); updated.ExpectedRevision != target.ExpectedRevision {
		t.Error("update without a key changed the directory revision")
	}

	withoutKey(delivery.MCPAuthorizationAttemptsCreate, protocol.CreateMCPAuthorizationAttemptRequest{Server: server})
	store := authorizationCrashStore(t, filepath.Join(cfg.stores.DataDirectory, "plugins", "data", connector.ID, "external-servers.db"))
	var attempts int
	if err := store.QueryRowContext(t.Context(), `SELECT count(*) FROM attempts WHERE name=?`, server.Name).Scan(&attempts); err != nil {
		t.Fatal(err)
	}
	if attempts != 0 {
		t.Errorf("authorization without a key recorded %d connector attempts", attempts)
	}

	withoutKey(delivery.MCPServersDelete, host.userServerTarget(server))
	if !exists(server.Name) {
		t.Error("deletion without a key withdrew the server")
	}
}

func TestCommittedMCPWithdrawalDetachesToolsEvenIfLocalSettlementFails(t *testing.T) {
	scenarios := []struct {
		name        string
		triggerName string
		trigger     string
		method      delivery.Name
		request     func(protocol.MCPDirectoryTarget) any
		removed     bool
	}{
		{
			name: "disable", triggerName: "withhold_directory_receipt",
			trigger: `CREATE TRIGGER withhold_directory_receipt BEFORE UPDATE OF payload ON idempotency_records
				WHEN OLD.key = 'withdraw-server'
				BEGIN SELECT RAISE(ABORT, 'withheld directory receipt'); END`,
			method: delivery.MCPServersUpdate,
			request: func(target protocol.MCPDirectoryTarget) any {
				return protocol.UpdateMCPServerRequest{MCPDirectoryTarget: target, Enabled: new(false)}
			},
		},
		{
			name: "remove", triggerName: "withhold_grant_removal",
			trigger: `CREATE TRIGGER withhold_grant_removal BEFORE DELETE ON mcp_launch_grants
				BEGIN SELECT RAISE(ABORT, 'withheld grant removal'); END`,
			method: delivery.MCPServersDelete,
			request: func(target protocol.MCPDirectoryTarget) any {
				return target
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
			request := scenario.request(runtime.userServerTarget(serverID))
			questionCrashDatabase(t, cfg.stores.DataDirectory, scenario.trigger)

			options := protocol.CallMetadata{
				IdempotencyKey: "withdraw-server", RequestMeta: protocol.RequestMeta{ProtocolVersion: new(protocol.Version)},
			}
			result := runtime.endpoint.Invoke(t.Context(), scenario.method, request, options)
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
			result = runtime.endpoint.Invoke(t.Context(), scenario.method, request, options)
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
		MCPDirectoryTarget: host.userServerTarget(id), Connection: &protocol.MCPConnectionInput{Type: protocol.MCPTransportStreamableHTTP, URL: replacement},
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
