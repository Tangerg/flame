package pluginservices

import (
	"context"
	"encoding/json/jsontext"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/qimu"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

func TestManagedCatalogProtectsCallsWithoutRetainingAnIdleUse(t *testing.T) {
	name := testsupport.ServerName("owned")
	release := testsupport.Release(t, "owned", plugin.Declaration{Name: "example.owned", Servers: []plugin.Server{{Name: name, Transport: mcpserver.TransportStdio, Command: "backend"}}})
	installation, err := plugin.New(testsupport.InstallationID(t), "original source", release)
	if err != nil {
		t.Fatal(err)
	}
	if err := installation.Enable(release); err != nil {
		t.Fatal(err)
	}
	source, err := installation.ServerSource(release, name)
	if err != nil {
		t.Fatal(err)
	}
	remote := sdk.NewServer(&sdk.Implementation{Name: "owned-call"}, nil)
	backend, err := qimumcp.NewBackend(remote)
	if err != nil {
		t.Fatal(err)
	}
	if err := backend.Publish(); err != nil {
		t.Fatal(err)
	}
	entered, releaseCall := make(chan struct{}), make(chan struct{})
	finish := sync.OnceFunc(func() { close(releaseCall) })
	defer finish()
	var calls atomic.Int32
	remote.AddTool(&sdk.Tool{Name: "wait", InputSchema: jsontext.Value(`{"type":"object"}`)}, func(ctx context.Context, _ *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
		if calls.Add(1) != 1 {
			return nil, errors.New("stale catalog reached a retired implementation")
		}
		close(entered)
		select {
		case <-releaseCall:
			return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: "original call"}}}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	})
	endpoint := httptest.NewServer(sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return remote }, nil))
	t.Cleanup(endpoint.Close)
	callbacks := qimumcp.NewCallbacks()
	t.Cleanup(func() { _ = callbacks.Close() })
	store := &toolInstallationStore{snapshot: plugin.Snapshot{Installation: installation, Selected: release}}
	managed := newManagedFixture(t, store, mcpserver.Server{Source: source, Name: name, Enabled: true, Transport: mcpserver.TransportStreamableHTTP, URL: endpoint.URL}, callbacks, nil)
	var tools []toolcontract.Tool
	managed.Pool.SetToolSink(func(current []toolcontract.Tool, _ []mcpserver.ID) { tools = current })
	if len(tools) != 1 {
		t.Fatalf("catalog = %d tools", len(tools))
	}
	bound, err := toolcontract.Bind(tools[0])
	if err != nil {
		t.Fatal(err)
	}
	invocation, err := bound.Contract().Prepare(chat.ToolCall{ID: "original", Name: bound.Contract().Definition().Name, Arguments: `{}`}, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx := operation.Adopt(t.Context(), "original-call", extension.Model, "run_original")
	done := make(chan error, 1)
	go func() {
		_, err := bound.Call(ctx, invocation)
		done <- err
	}()
	select {
	case <-entered:
	case err := <-done:
		t.Fatalf("call returned before entering its implementation: %v", err)
	case <-t.Context().Done():
		t.Fatal(t.Context().Err())
	}
	id := qimu.InstallationID(installation.ID().String())
	if err := managed.Host.Uninstall(t.Context(), id); !errors.Is(err, qimu.ErrInUse) {
		t.Fatalf("active call lost its exact Use: %v", err)
	}
	finish()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if err := managed.Host.Uninstall(t.Context(), id); err != nil {
		t.Fatalf("idle catalog retained a Use after the original call closed: %v", err)
	}
	if _, err := bound.Call(ctx, invocation); err == nil || calls.Load() != 1 {
		t.Fatalf("stale observation dispatched after removal: calls=%d, %v", calls.Load(), err)
	}
}

func TestManagedReconnectWaitsForOriginalUseBeforeStartingReplacement(t *testing.T) {
	name := testsupport.ServerName("reconnect")
	release := testsupport.Release(t, "reconnect", plugin.Declaration{Name: "example.reconnect", Servers: []plugin.Server{{Name: name, Transport: mcpserver.TransportStdio, Command: "backend"}}})
	installation, err := plugin.New(testsupport.InstallationID(t), "original source", release)
	if err != nil {
		t.Fatal(err)
	}
	if err := installation.Enable(release); err != nil {
		t.Fatal(err)
	}
	source, err := installation.ServerSource(release, name)
	if err != nil {
		t.Fatal(err)
	}
	remote := sdk.NewServer(&sdk.Implementation{Name: "reconnect-owner"}, nil)
	backend, err := qimumcp.NewBackend(remote)
	if err != nil {
		t.Fatal(err)
	}
	if err := backend.Publish(); err != nil {
		t.Fatal(err)
	}
	endpoint := httptest.NewServer(sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return remote }, nil))
	t.Cleanup(endpoint.Close)
	callbacks := qimumcp.NewCallbacks()
	t.Cleanup(func() { _ = callbacks.Close() })
	store := &toolInstallationStore{snapshot: plugin.Snapshot{Installation: installation, Selected: release}}
	server := mcpserver.Server{Source: source, Name: name, Enabled: true, Transport: mcpserver.TransportStreamableHTTP, URL: endpoint.URL}
	managed := newManagedFixture(t, store, server, callbacks, nil)
	borrowed, err := managed.Pool.BorrowSession(t.Context(), server.ID())
	if err != nil {
		t.Fatal(err)
	}
	defer borrowed.Close()
	original := borrowed.Session()
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	attempt, err := managed.Pool.BeginConnect(ctx, server.ID())
	if err != nil {
		t.Fatal(err)
	}
	if err := attempt.Run(); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("reconnect did not join the original instance's accepted Use: %v", err)
	}
	if _, err := borrowed.Activation(); err != nil {
		t.Fatalf("canceling the reconnect waiter revoked the original Use: %v", err)
	}
	if err := borrowed.Close(); err != nil {
		t.Fatal(err)
	}
	if err := managed.Host.WaitRetired(t.Context(), qimu.InstallationID(installation.ID().String())); err != nil {
		t.Fatal(err)
	}
	attempt, err = managed.Pool.BeginConnect(t.Context(), server.ID())
	if err != nil {
		t.Fatal(err)
	}
	if err := attempt.Run(); err != nil {
		t.Fatal(err)
	}
	replacement, err := managed.Pool.BorrowSession(t.Context(), server.ID())
	if err != nil {
		t.Fatal(err)
	}
	defer replacement.Close()
	if replacement.Session() == original {
		t.Fatal("reconnect published the retired original session")
	}
}
