package terminal

import (
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/flame/apps/cli/internal/adapter/runtimebinding"
	"github.com/Tangerg/flame/apps/cli/internal/application/integration/mcp"
	"github.com/Tangerg/flame/apps/cli/internal/application/mutation"
	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/oolong/ui/core/input"
)

func TestTerminalRecoversTheOriginalMCPAuthorization(t *testing.T) {
	connection, session := openSessionNavigationRuntime(t)
	remote := httptest.NewServer(http.HandlerFunc(serveAuthorizationFixture))
	t.Cleanup(remote.Close)
	server := mcp.UserServer("authorization-evidence")
	if err := connection.CreateServer(t.Context(), mutation.NewCommandID(), protocol.MCPServerCandidate{
		Name: server.Name, Enabled: true,
		Connection:       protocol.MCPConnectionInput{Type: protocol.MCPTransportStreamableHTTP, URL: remote.URL},
		HandshakeTimeout: protocol.MCPHandshakeTimeout{Type: protocol.MCPHandshakeUnbounded},
	}); err != nil {
		t.Fatal(err)
	}

	references := make(chan mcp.AuthorizationReference, 2)
	service := &lostMCPAuthorizationReply{Connection: connection, references: references}
	config := sessionNavigationConfig(connection, connection, session)
	config.MCP = service
	host, _, _ := runUIFromConfig(t, config)
	host.Shows(t, "Ask flame")
	command := func(text string) {
		t.Helper()
		if !host.Type(text) || !host.Press(input.Enter) {
			t.Fatalf("terminal refused %q", text)
		}
	}
	command("/mcp")
	host.Shows(t, server.Name+" · connected")
	if !host.Press(input.Esc) {
		t.Fatal("terminal refused to close the MCP directory")
	}
	host.Shows(t, "Ask flame")
	start := "/" + mcpAuthorizationCommand + " " + server.Name
	command(start)
	host.Shows(t, "repeat "+start)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	readReference := func() mcp.AuthorizationReference {
		t.Helper()
		select {
		case reference := <-references:
			return reference
		case <-ctx.Done():
			t.Fatal("Runtime did not acknowledge the authorization", context.Cause(ctx))
			return mcp.AuthorizationReference{}
		}
	}
	initial := readReference()
	command(start)
	recovered := readReference()
	if recovered != initial {
		t.Fatalf("retry created authorization %q after Runtime accepted %q", recovered.ID, initial.ID)
	}
	host.Shows(t, "succeeded")
	attempt, err := connection.GetAuthorization(ctx, initial)
	if err != nil || attempt.Status.Type != protocol.MCPAuthorizationAttemptSucceeded {
		t.Fatalf("Runtime authorization %q did not settle: %+v, %v", initial.ID, attempt, err)
	}
	if !host.Press(input.Esc) {
		t.Fatal("terminal refused to close the recovered authorization")
	}
	host.Shows(t, "Ask flame")
	command(start)
	next := readReference()
	if next == initial || next.Server != server {
		t.Fatalf("settled command did not release the server for a new authorization: %+v", next)
	}
	host.Shows(t, "succeeded")
}

type lostMCPAuthorizationReply struct {
	*runtimebinding.Connection
	references chan<- mcp.AuthorizationReference
	injected   atomic.Bool
}

func (l *lostMCPAuthorizationReply) StartAuthorization(ctx context.Context, commandID authoring.CommandID, server protocol.MCPServerID) (mcp.AuthorizationReference, error) {
	reference, err := l.Connection.StartAuthorization(ctx, commandID, server)
	if err != nil {
		return reference, err
	}
	select {
	case l.references <- reference:
	case <-ctx.Done():
		return mcp.AuthorizationReference{}, context.Cause(ctx)
	}
	if l.injected.CompareAndSwap(false, true) {
		return mcp.AuthorizationReference{}, fmt.Errorf("lost authorization acknowledgement: %w", conversation.ErrDisconnected)
	}
	return reference, nil
}

func serveAuthorizationFixture(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var message struct {
		ID     jsontext.Value `json:"id"`
		Method string         `json:"method"`
		Params struct {
			ProtocolVersion string `json:"protocolVersion"`
		} `json:"params"`
	}
	if err := json.UnmarshalRead(r.Body, &message); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if message.ID == nil {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	var result jsontext.Value
	switch message.Method {
	case "initialize":
		result = jsontext.Value(fmt.Appendf(nil, `{"protocolVersion":%q,"capabilities":{"tools":{}},"serverInfo":{"name":"authorization-fixture","version":"1"}}`, message.Params.ProtocolVersion))
	case "tools/list":
		result = jsontext.Value(`{"tools":[]}`)
	default:
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%s,"result":%s}`, message.ID, result)
}
