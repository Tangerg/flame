package bootstrap

import (
	"context"
	"database/sql/driver"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	modernsqlite "modernc.org/sqlite"
)

func TestAcceptedMCPDisableSurvivesCallerCancellation(t *testing.T) {
	waiting, resume := make(chan struct{}), make(chan struct{})
	release := sync.OnceFunc(func() { close(resume) })
	defer release()
	if err := modernsqlite.RegisterScalarFunction("test_mcp_directory_acknowledgement", 0,
		func(*modernsqlite.FunctionContext, []driver.Value) (driver.Value, error) {
			close(waiting)
			<-resume
			return nil, nil
		}); err != nil {
		t.Fatal(err)
	}

	remote := poMCPRemote(t, "record")
	cfg := poMCPConfig(t)
	runtime := poMCPOpen(t, cfg)
	installExternalConnector(runtime)
	serverID := runtime.createUserServer(protocol.MCPServerCandidate{
		Name: "disable-after-commit", Enabled: true,
		Connection:       protocol.MCPConnectionInput{Type: protocol.MCPTransportStreamableHTTP, URL: remote},
		HandshakeTimeout: protocol.MCPHandshakeTimeout{Type: protocol.MCPHandshakeUnbounded},
	}, "create-before-disable")
	runtime.poMCPTools(&serverID, func(tools []protocol.MCPTool) bool { return len(tools) == 1 })
	questionCrashDatabase(t, cfg.stores.DataDirectory, `CREATE TRIGGER pause_mcp_directory_acknowledgement
		BEFORE UPDATE OF payload ON idempotency_records
		WHEN OLD.key = 'disable-once'
		BEGIN SELECT test_mcp_directory_acknowledgement(); END`)

	request := protocol.UpdateMCPServerRequest{Server: serverID, Enabled: new(false)}
	options := protocol.CallMetadata{
		IdempotencyKey: "disable-once", RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.Version},
	}
	callCtx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan protocol.OperationResult, 1)
	go func() { done <- runtime.endpoint.Invoke(callCtx, delivery.MCPServersUpdate, request, options) }()
	select {
	case <-waiting:
	case result := <-done:
		t.Fatalf("disable finished before directory acknowledgement: %+v", result)
	case <-time.After(lifecycleWaitBudget(t)):
		t.Fatal("disable did not reach directory acknowledgement")
	}
	assertWithdrawn := func() {
		t.Helper()
		tools := runtime.must(delivery.MCPToolsList, protocol.MCPListToolsRequest{Server: &serverID}, "").(*protocol.Page[protocol.MCPTool])
		if len(tools.Data) != 0 {
			t.Errorf("disabled server still advertises tools: %+v", tools.Data)
		}
	}
	assertWithdrawn()
	cancel()
	release()
	select {
	case <-done:
	case <-time.After(lifecycleWaitBudget(t)):
		t.Fatal("committed disable did not settle")
	}

	recorded := runtime.endpoint.Invoke(t.Context(), delivery.MCPServersUpdate, request, options)
	if recorded.Failure != nil || recorded.Value != (struct{}{}) {
		t.Fatalf("replayed disable = %+v, %v", recorded.Value, recorded.Failure)
	}
	servers, err := runtime.api.ListMCPServers(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(servers.Data, func(server protocol.MCPServer) bool {
		return server.ID == serverID && server.Status.Type == protocol.MCPServerDisabled
	}) {
		t.Fatalf("disabled server missing from current directory: %+v", servers.Data)
	}
	assertWithdrawn()
}
