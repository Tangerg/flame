package bootstrap

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func TestMCPReconciliationPreservesAnAdmittedAuthorization(t *testing.T) {
	handler := poMCPRemoteHandler(t, "record")
	entered, release := make(chan struct{}), make(chan struct{})
	observed := sync.OnceFunc(func() { close(entered) })
	finish := sync.OnceFunc(func() { close(release) })
	defer finish()
	var hold atomic.Bool
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if hold.Load() && req.Method == http.MethodPost {
			observed()
			select {
			case <-release:
			case <-req.Context().Done():
				return
			}
		}
		handler.ServeHTTP(w, req)
	}))
	t.Cleanup(remote.Close)

	r := poMCPOpen(t, poMCPConfig(t))
	connector := installExternalConnector(r)
	server := r.createUserServer(protocol.MCPServerCandidate{
		Name: "authorization-reconcile", Enabled: true,
		Connection:       protocol.MCPConnectionInput{Type: protocol.MCPTransportStreamableHTTP, URL: remote.URL},
		HandshakeTimeout: protocol.MCPHandshakeTimeout{Type: protocol.MCPHandshakeUnbounded},
	}, "create-server-for-reconciliation")
	r.poMCPTools(&server, func(tools []protocol.MCPTool) bool { return len(tools) == 1 && tools[0].Name == "record" })

	hold.Store(true)
	ref := r.must(delivery.MCPAuthorizationAttemptsCreate, protocol.CreateMCPAuthorizationAttemptRequest{Server: server}, "authorize-during-reconciliation").(*protocol.MCPAuthorizationAttemptRef)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatalf("authorization did not open its transport: %v", ctx.Err())
	}

	installation, err := identity.ParseInstallation(connector.ID)
	if err != nil {
		t.Fatal(err)
	}
	origin, err := mcpserver.InstallationOrigin(installation)
	if err != nil {
		t.Fatal(err)
	}
	if len(connector.Selected.Servers) != 1 {
		t.Fatalf("expected a single Connector backend: %+v", connector.Selected.Servers)
	}
	id, err := mcpserver.NewID(origin, testsupport.ServerName(connector.Selected.Servers[0].Name))
	if err != nil {
		t.Fatal(err)
	}
	if err := r.host.application.delivery.MCP.ReconcileInstallation(ctx, []mcpserver.ID{id}); err != nil {
		t.Fatal(err)
	}
	finish()

	if attempt := awaitMCPAuthorization(t, r, ref.AttemptID); attempt.Status.Type != protocol.MCPAuthorizationAttemptSucceeded {
		t.Fatalf("the same configuration interrupted its accepted authorization: %+v", attempt)
	}
}
