package bootstrap

import (
	"context"
	"slices"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/delivery"
)

func TestMCPToolExposureAndItsReceiptCommitTogether(t *testing.T) {
	const setExposure delivery.Name = "mcp.tools.setExposure"
	const toolName = "record"
	const disableKey = "disable-original-tool"
	remote := poMCPRemote(t, toolName)
	cfg := poMCPConfig(t)
	instance := assembleTestRuntime(t, cfg)
	host := &poMCPRuntime{t: t, host: instance}
	t.Cleanup(host.close)
	api, err := pluginProtocolHandler(instance, cfg)
	if err != nil {
		t.Fatal(err)
	}
	committed, release := make(chan struct{}), make(chan struct{})
	commandID := operation.Identity(operation.Bind(t.Context(), cfg.stores.IdempotencyNamespace.String(), disableKey))
	if err := startTestRuntimeWithReplayStore(t, instance, api, cfg.stores, pausedReceiptCommit{cfg.stores.Idempotency, commandID, committed, release}); err != nil {
		t.Fatal(err)
	}
	host.api, host.endpoint = api, instance.Endpoint()
	installExternalConnector(host)
	candidate := protocol.MCPServerCandidate{
		Name: "exposure.commit", Enabled: true,
		Connection:       protocol.MCPConnectionInput{Type: protocol.MCPTransportStreamableHTTP, URL: remote},
		HandshakeTimeout: protocol.MCPHandshakeTimeout{Type: protocol.MCPHandshakeUnbounded},
	}
	serverID := host.createUserServer(candidate, "create-exposure-server")
	candidate.Name = "exposure_commit"
	other := host.createUserServer(candidate, "create-colliding-server")
	host.poMCPTools(&other, func(tools []protocol.MCPTool) bool {
		return len(tools) == 1 && len(tools[0].NameConflicts) == 1
	})

	exposure := func(want ...string) {
		t.Helper()
		result, err := host.api.GetMCPToolExposure(t.Context(), protocol.MCPServerRequest{Server: serverID})
		if err != nil || result == nil || !slices.Equal(result.DisabledTools, want) {
			t.Fatalf("disabled tools = %+v, %v, want %v", result, err, want)
		}
	}
	disable := protocol.SetMCPToolExposureRequest{Server: serverID, Name: toolName, Disabled: true}
	allowReceipt := refuseCommandReceipt(t, cfg.stores.DataDirectory)
	refused := host.endpoint.Invoke(t.Context(), setExposure, disable, protocol.CallMetadata{IdempotencyKey: disableKey})
	allowReceipt()
	if refused.Failure == nil {
		t.Fatal("disabled the tool despite a refused receipt")
	}
	exposure()
	ctx, cancel := context.WithTimeout(t.Context(), lifecycleWaitBudget(t))
	defer cancel()
	written := make(chan *protocol.Failure, 1)
	go func() {
		result := host.endpoint.Invoke(ctx, setExposure, disable, protocol.CallMetadata{IdempotencyKey: disableKey})
		written <- result.Failure
	}()
	select {
	case <-committed:
	case failure := <-written:
		t.Fatalf("disable tool ended without reaching its commit: %v", failure)
	case <-ctx.Done():
		t.Fatal("tool exposure did not commit")
	}
	exposure(toolName)
	tools := host.must(delivery.MCPToolsList, protocol.MCPListToolsRequest{Server: &other}, "").(*protocol.Page[protocol.MCPTool])
	close(release)
	if failure := <-written; failure != nil {
		t.Fatal(failure)
	}
	if len(tools.Data) != 1 || len(tools.Data[0].NameConflicts) != 0 {
		t.Fatalf("tool catalog after committing exposure = %+v, want one enabled tool without a conflict", tools.Data)
	}

	host.must(setExposure, protocol.SetMCPToolExposureRequest{Server: serverID, Name: toolName}, "enable-tool")
	host.must(setExposure, disable, disableKey)
	exposure()
}
