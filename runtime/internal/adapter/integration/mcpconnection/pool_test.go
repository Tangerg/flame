package mcpconnection

import (
	"context"
	"encoding/json/jsontext"
	"errors"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	mcpapp "github.com/Tangerg/flame/runtime/internal/application/integration/mcp"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/infra/integration/mcp"
	sdkmcp "github.com/Tangerg/go-sdk/mcp"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

func TestConfigFromServerMapsEachSupportedTransport(t *testing.T) {
	timeout, err := mcpserver.NewHandshakeTimeout(time.Second)
	if err != nil {
		t.Fatalf("NewHandshakeTimeout: %v", err)
	}
	tests := []struct {
		name string
		in   mcpserver.Server
		want mcp.Transport
	}{
		{
			name: "streamable http",
			in: mcpserver.Server{
				Source: mcpserver.UserSource(),
				Name:   testsupport.ServerName("remote"), Transport: mcpserver.TransportStreamableHTTP,
				URL: "https://mcp.example/tools", Authorization: "Bearer token",
				Headers: map[string]string{"X-Trace": "enabled"}, HandshakeTimeout: timeout,
			},
			want: mcp.TransportHTTP,
		},
		{
			name: "stdio",
			in: mcpserver.Server{
				Source: mcpserver.UserSource(),
				Name:   testsupport.ServerName("local"), Transport: mcpserver.TransportStdio,
				Command: "mcp-server", Args: []string{"--stdio"},
				Env: map[string]string{"B": "two", "A": "one"}, Dir: "/tmp", HandshakeTimeout: timeout,
			},
			want: mcp.TransportStdio,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := configFromServer(test.in)
			if err != nil {
				t.Fatalf("configFromServer: %v", err)
			}
			if got.Transport != test.want {
				t.Fatalf("transport = %v, want %v", got.Transport, test.want)
			}
			if err := got.Validate(); err != nil {
				t.Fatalf("mapped config invalid: %v", err)
			}
		})
	}
}

func TestConfigFromServerRejectsInvalidDomainValue(t *testing.T) {
	_, err := configFromServer(mcpserver.Server{
		Source: mcpserver.UserSource(),
		Name:   testsupport.ServerName("broken"), Transport: mcpserver.Transport("websocket"), URL: "https://mcp.example",
	})
	if err == nil {
		t.Fatal("configFromServer error = nil, want invalid transport")
	}
}

func TestRefusedConnectionWithdrawsTheLiveSession(t *testing.T) {
	for _, tt := range []struct {
		name    string
		refusal error
		detach  bool
	}{
		{name: "unverifiable source", refusal: errors.New("release bytes changed")},
		{name: "removed source", refusal: mcpapp.ErrUnknownServer, detach: true},
		{name: "disabled source", refusal: mcpapp.ErrServerDisabled, detach: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			remote := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "refused-owner"}, nil)
			remote.AddTool(&sdkmcp.Tool{Name: "read", InputSchema: jsontext.Value(`{"type":"object"}`)}, func(context.Context, *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
				return &sdkmcp.CallToolResult{}, nil
			})
			transport := httptest.NewServer(sdkmcp.NewStreamableHTTPHandler(func(*http.Request) *sdkmcp.Server { return remote }, nil))
			t.Cleanup(transport.Close)
			server := mcpserver.Server{Source: mcpserver.UserSource(), Name: testsupport.ServerName("remote"), Enabled: true, Transport: mcpserver.TransportStreamableHTTP, URL: transport.URL}
			registry := &mutableSourceRegistry{server: server.Clone()}
			pool, initial, err := Open(t.Context(), t.Context(), []mcpserver.Server{server}, nil, registry)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := pool.Shutdown(context.WithoutCancel(t.Context())); err != nil {
					t.Error(err)
				}
			})
			if len(initial) != 1 {
				t.Fatalf("initial tools = %d", len(initial))
			}
			published := []toolcontract.Tool{nil}
			pool.SetToolSink(func(tools []toolcontract.Tool) { published = tools })
			registry.refuse(tt.refusal)
			for operation, connect := range map[string]func() error{
				"configure": func() error { return pool.Configure(t.Context(), server.ID()) },
				"reconnect": func() error { return pool.Reconnect(t.Context(), server.ID()) },
			} {
				if err := connect(); !errors.Is(err, tt.refusal) {
					t.Fatalf("%s = %v, want the refusal", operation, err)
				}
				if len(published) != 0 {
					t.Fatalf("%s left %d tools serving after refusal", operation, len(published))
				}
				statuses := pool.Statuses()
				if tt.detach {
					if len(statuses) != 0 {
						t.Fatalf("%s kept a withdrawn source live: %+v", operation, statuses)
					}
					continue
				}
				want := []mcpserver.ConnectionStatus{{Server: server.ID(), State: mcpserver.ConnectionFailed, Failure: mcpserver.FailureConfiguration}}
				if !slices.Equal(statuses, want) {
					t.Fatalf("%s status = %+v, want %+v", operation, statuses, want)
				}
			}
		})
	}
}

func TestSupersededRefusalRecordsNothing(t *testing.T) {
	server := mcpserver.Server{Source: mcpserver.UserSource(), Name: testsupport.ServerName("remote"), Enabled: true, Transport: mcpserver.TransportStreamableHTTP, URL: "https://mcp.example/tools"}
	registry := &mutableSourceRegistry{server: server.Clone(), refusal: errors.New("unreadable source")}
	pool, _, err := Open(t.Context(), t.Context(), nil, nil, registry)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pool.Shutdown(context.WithoutCancel(t.Context())) })
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := pool.Reconnect(ctx, server.ID()); !errors.Is(err, context.Canceled) {
		t.Fatalf("superseded reconnect = %v", err)
	}
	if statuses := pool.Statuses(); len(statuses) != 0 {
		t.Fatalf("a superseded refusal reached status: %+v", statuses)
	}
}
