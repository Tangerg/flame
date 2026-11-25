package bootstrap

import (
	"context"
	"encoding/json/jsontext"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func TestMCPReconnectRequiresACommittedReceipt(t *testing.T) {
	const toolName = "record"
	remote := sdk.NewServer(&sdk.Implementation{Name: "reconnect-fixture"}, nil)
	remote.AddTool(&sdk.Tool{Name: toolName, InputSchema: jsontext.Value(`{"type":"object"}`)}, func(context.Context, *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
		return &sdk.CallToolResult{}, nil
	})
	mcpHTTP := sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return remote }, nil)
	type transportGate struct {
		arrived chan struct{}
		release chan struct{}
	}
	var blocked atomic.Pointer[transportGate]
	blockTransport := func() (*transportGate, func()) {
		gate := &transportGate{arrived: make(chan struct{}, 1), release: make(chan struct{})}
		blocked.Store(gate)
		return gate, sync.OnceFunc(func() { close(gate.release) })
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if gate := blocked.Load(); gate != nil && req.Method == http.MethodPost {
			select {
			case gate.arrived <- struct{}{}:
			default:
			}
			select {
			case <-req.Context().Done():
				return
			case <-gate.release:
			}
		}
		mcpHTTP.ServeHTTP(w, req)
	}))
	t.Cleanup(upstream.Close)

	cfg := poMCPConfig(t)
	host := poMCPOpen(t, cfg)
	installExternalConnector(host)
	server := host.must(delivery.MCPServersCreate, protocol.MCPServerCandidate{
		Name: "reconnect-receipt", Enabled: true,
		Connection:       protocol.MCPConnectionInput{Type: protocol.MCPTransportStreamableHTTP, URL: upstream.URL},
		HandshakeTimeout: protocol.MCPHandshakeTimeout{Type: protocol.MCPHandshakeUnbounded},
	}, "create-reconnect-server").(*protocol.MCPServer)
	ready := func(tools []protocol.MCPTool) bool { return len(tools) == 1 && tools[0].Name == toolName }
	host.poMCPTools(&server.ID, ready)
	requireVisible := func(phase string) {
		t.Helper()
		tools := host.must(delivery.MCPToolsList, protocol.MCPListToolsRequest{Server: &server.ID}, "").(*protocol.Page[protocol.MCPTool])
		if !ready(tools.Data) {
			t.Fatalf("%s tools = %+v, want the current connection", phase, tools.Data)
		}
	}

	gate, release := blockTransport()
	defer release()
	request := protocol.MCPServerRequest{Server: server.ID}
	const key = "reconnect-original-server"
	allowReceipt := refuseCommandReceipt(t, cfg.stores.DataDirectory)
	refused := host.endpoint.Invoke(t.Context(), delivery.MCPServersReconnect, request, protocol.CallMetadata{IdempotencyKey: key})
	allowReceipt()
	if refused.Failure == nil {
		t.Fatal("accepted reconnect despite a refused receipt")
	}
	requireVisible("after a refused reconnect receipt")

	host.must(delivery.MCPServersReconnect, request, key)
	waitForSignal(t, gate.arrived, "the accepted reconnect transport")
	release()
	host.poMCPTools(&server.ID, ready)

	_, releaseReplay := blockTransport()
	defer releaseReplay()
	host.must(delivery.MCPServersReconnect, request, key)
	requireVisible("after replaying the accepted reconnect")
}
