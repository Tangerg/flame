package mcp

import (
	"context"
	"encoding/json/jsontext"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	mcpapp "github.com/Tangerg/flame/runtime/internal/application/integration/mcp"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	sdkmcp "github.com/Tangerg/go-sdk/mcp"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

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
			pool, err := Open(t.Context(), t.Context(), []mcpserver.Server{server}, nil, registry, nil, nil, nil, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := pool.Shutdown(context.WithoutCancel(t.Context())); err != nil {
					t.Error(err)
				}
			})
			var initial []toolcontract.Tool
			awaitPoolStartup(t, pool)
			pool.SetToolSink(func(tools []toolcontract.Tool, _ []mcpserver.ID) { initial = tools })
			if len(initial) != 1 {
				t.Fatalf("initial tools = %d", len(initial))
			}
			published := []toolcontract.Tool{nil}
			pool.SetToolSink(func(tools []toolcontract.Tool, _ []mcpserver.ID) { published = tools })
			registry.refuse(tt.refusal)
			for operation, connect := range map[string]func() error{
				"configure": func() error { return connectPool(pool, t.Context(), server.ID()) },
				"reconnect": func() error { return connectPool(pool, t.Context(), server.ID()) },
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
	pool, err := Open(t.Context(), t.Context(), nil, nil, registry, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pool.Shutdown(context.WithoutCancel(t.Context())) })
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := connectPool(pool, ctx, server.ID()); !errors.Is(err, context.Canceled) {
		t.Fatalf("superseded reconnect = %v", err)
	}
	if statuses := pool.Statuses(); len(statuses) != 0 {
		t.Fatalf("a superseded refusal reached status: %+v", statuses)
	}
}

func connectPool(pool *Pool, ctx context.Context, name mcpserver.ID) error {
	attempt, err := pool.BeginConnect(ctx, name)
	if err != nil {
		return err
	}
	defer attempt.Close()
	return attempt.Run()
}

func authorizePool(pool *Pool, ctx context.Context, name mcpserver.ID) error {
	attempt, err := pool.BeginAuthorize(ctx, name)
	if err != nil {
		return err
	}
	defer attempt.Close()
	return attempt.Run()
}

func awaitPoolStartup(t *testing.T, pool *Pool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		pending := false
		for _, status := range pool.Statuses() {
			if status.State == mcpserver.ConnectionConnecting {
				pending = true
			}
		}
		if !pending {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("initial connection did not settle")
		}
		time.Sleep(time.Millisecond)
	}
}
