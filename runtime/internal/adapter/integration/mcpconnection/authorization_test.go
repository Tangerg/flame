package mcpconnection

import (
	"context"
	"encoding/json/jsontext"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"
	"time"

	mcpapp "github.com/Tangerg/flame/runtime/internal/application/integration/mcp"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	sdkmcp "github.com/Tangerg/go-sdk/mcp"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

type heldLaunchRegistry struct {
	*mutableSourceRegistry
	guard    context.Context
	captured chan struct{}
	resume   chan struct{}
}

type heldAuthorization struct{}

func (r *heldLaunchRegistry) Connection(ctx context.Context, name mcpserver.ID) (mcpapp.Launch, error) {
	launch, err := r.mutableSourceRegistry.Connection(ctx, name)
	if ctx.Value(heldAuthorization{}) != nil {
		close(r.captured)
		select {
		case <-r.resume:
		case <-r.guard.Done():
			return mcpapp.Launch{}, r.guard.Err()
		}
	}
	return launch, err
}

func TestSupersededAuthorizationPreservesReplacementConnection(t *testing.T) {
	ctx, bounded := context.WithTimeout(t.Context(), 5*time.Second)
	defer bounded()
	remote := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "local-server"}, nil)
	remote.AddTool(&sdkmcp.Tool{Name: "read", InputSchema: jsontext.Value(`{"type":"object"}`)}, func(context.Context, *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		return &sdkmcp.CallToolResult{}, nil
	})
	transport := httptest.NewServer(sdkmcp.NewStreamableHTTPHandler(func(*http.Request) *sdkmcp.Server { return remote }, nil))
	defer transport.Close()
	server := mcpserver.Server{Source: mcpserver.UserSource(), Name: testsupport.ServerName("remote"), Enabled: true, Transport: mcpserver.TransportStreamableHTTP, URL: transport.URL}
	registry := &heldLaunchRegistry{mutableSourceRegistry: &mutableSourceRegistry{server: server.Clone()}, guard: ctx, captured: make(chan struct{}), resume: make(chan struct{})}
	pool, err := Open(ctx, ctx, []mcpserver.Server{server}, nil, registry)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := pool.Shutdown(context.WithoutCancel(ctx)); err != nil {
			t.Error(err)
		}
	}()
	var initial []toolcontract.Tool
	pool.SetToolSink(func(tools []toolcontract.Tool) { initial = tools })
	if len(initial) != 1 {
		t.Fatalf("initial tools=%d", len(initial))
	}
	resume := sync.OnceFunc(func() { close(registry.resume) })
	defer resume()
	oldCtx, supersede := context.WithCancel(context.WithValue(ctx, heldAuthorization{}, true))
	defer supersede()
	done := make(chan error, 1)
	go func() { done <- pool.Authorize(oldCtx, server.ID()) }()
	select {
	case <-registry.captured:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	supersede()
	if err := pool.Reconnect(ctx, server.ID()); err != nil {
		t.Fatal(err)
	}
	before := pool.Statuses()
	if len(before) != 1 || before[0].State != mcpserver.ConnectionConnected {
		t.Fatalf("replacement did not connect: %+v", before)
	}
	resume()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("old authorization=%v", err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	after := pool.Statuses()
	if !slices.Equal(before, after) {
		t.Fatalf("superseded authorization destroyed replacement: before=%+v after=%+v", before, after)
	}
}
