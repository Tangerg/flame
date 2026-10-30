package mcp

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

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func catalogRemote(t *testing.T, name, toolName string) (*sdk.Server, ServerConfig) {
	t.Helper()
	remote := sdk.NewServer(&sdk.Implementation{Name: name}, nil)
	remote.AddTool(&sdk.Tool{Name: toolName, InputSchema: jsontext.Value(`{"type":"object"}`)}, catalogResult)
	transport := httptest.NewServer(sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return remote }, nil))
	t.Cleanup(transport.Close)
	return remote, ServerConfig{Server: mcpserver.Server{Source: mcpserver.UserSource(), Name: testsupport.ServerName(name), Transport: mcpserver.TransportStreamableHTTP, URL: transport.URL}}
}
func catalogResult(context.Context, *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
	return &sdk.CallToolResult{}, nil
}

func catalogPublications(t *testing.T, owner *connections) <-chan []Executable {
	t.Helper()
	snapshots := make(chan []Executable, 32)
	owner.setToolSink(func(tools []Executable, _, _ []mcpserver.ID) { snapshots <- tools })
	return snapshots
}
func awaitCatalog(t *testing.T, snapshots <-chan []Executable, expected func([]Executable) bool) []Executable {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	for {
		select {
		case tools := <-snapshots:
			if expected(tools) {
				return tools
			}
		case <-ctx.Done():
			t.Fatal("catalog publication did not reach its expected observation")
			return nil
		}
	}
}

func TestRemoteToolChangesReplaceTheCurrentCatalog(t *testing.T) {
	remote, config := catalogRemote(t, "files", "read")
	owner := newTestConnections(t)
	t.Cleanup(func() {
		if err := owner.shutdown(context.WithoutCancel(t.Context())); err != nil {
			t.Error(err)
		}
	})
	snapshots := catalogPublications(t, owner)
	if err := connectLaunch(owner, t.Context(), mustLaunch(t, config)); err != nil {
		t.Fatal(err)
	}
	initial := awaitCatalog(t, snapshots, func(tools []Executable) bool { return len(tools) == 1 })
	if !initial[0].Current() {
		t.Fatal("control catalog was not current")
	}
	remote.AddTool(&sdk.Tool{Name: "read", Description: "updated schema", InputSchema: jsontext.Value(`{"type":"object","properties":{"value":{"type":"string"}}}`)}, catalogResult)
	updated := awaitCatalog(t, snapshots, func(tools []Executable) bool {
		return len(tools) == 1 && tools[0].Definition().Description == "updated schema"
	})
	if initial[0].Definition().Description != "" || initial[0].Current() {
		t.Fatal("frozen catalog silently adopted changed tool material")
	}
	if borrowed, err := initial[0].BorrowSession(t.Context()); borrowed != nil || !errors.Is(err, ErrUnavailable) {
		t.Fatalf("obsolete catalog borrow = %v, %v", borrowed, err)
	}
	if !updated[0].Current() {
		t.Fatal("replacement catalog is not dispatchable")
	}
	remote.RemoveTools("read")
	remote.AddTool(&sdk.Tool{Name: "write", InputSchema: jsontext.Value(`{"type":"object"}`)}, catalogResult)
	current := awaitCatalog(t, snapshots, func(tools []Executable) bool { return len(tools) == 1 && tools[0].Definition().Name == "files_write" })
	if updated[0].Current() || !current[0].Current() {
		t.Fatal("removed tool retained current authority")
	}
	statuses := owner.statuses()
	if len(statuses) != 1 || statuses[0].State != mcpserver.ConnectionConnected || statuses[0].ToolCount != 1 {
		t.Fatalf("status and catalog disagree: %+v", statuses)
	}
}

func TestInvalidToolRefreshWithdrawsOnlyItsOwnSource(t *testing.T) {
	remote, config := catalogRemote(t, "files", "read")
	_, sibling := catalogRemote(t, "docs", "search")
	owner := newTestConnections(t)
	t.Cleanup(func() {
		if err := owner.shutdown(context.WithoutCancel(t.Context())); err != nil {
			t.Error(err)
		}
	})
	snapshots := catalogPublications(t, owner)
	for _, source := range []ServerConfig{config, sibling} {
		if err := connectLaunch(owner, t.Context(), mustLaunch(t, source)); err != nil {
			t.Fatal(err)
		}
	}
	initial := awaitCatalog(t, snapshots, func(tools []Executable) bool { return len(tools) == 2 })
	remote.AddTool(&sdk.Tool{Name: "read", InputSchema: jsontext.Value(`{"type":"object","properties":{"value":{"$ref":"#/$defs/missing"}}}`)}, catalogResult)
	current := awaitCatalog(t, snapshots, func(tools []Executable) bool { return len(tools) == 1 && tools[0].Definition().Name == "docs_search" })
	if !current[0].Current() {
		t.Fatal("unrelated source lost dispatch authority")
	}
	for _, prior := range initial {
		if prior.SourceConfig().ID() == config.ID() && prior.Current() {
			t.Fatal("unverified source kept serving its stale catalog")
		}
	}
	for _, status := range owner.statuses() {
		if status.Server == config.ID() && (status.State != mcpserver.ConnectionFailed || status.Failure != mcpserver.FailureToolDiscovery || status.ToolCount != 0) {
			t.Fatalf("refresh failure misrepresented as complete empty catalog: %+v", status)
		}
	}
}

func TestToolNotificationsCoalesceAndDiscardAnObsoleteRefresh(t *testing.T) {
	remote, config := catalogRemote(t, "files", "read")
	var queries atomic.Int32
	entered, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	remote.AddReceivingMiddleware(func(next sdk.MethodHandler) sdk.MethodHandler {
		return func(ctx context.Context, method string, request sdk.Request) (sdk.Result, error) {
			if method != "tools/list" {
				return next(ctx, method, request)
			}
			index := queries.Add(1)
			result, err := next(ctx, method, request)
			if index != 2 || err != nil {
				return result, err
			}
			original := result.(*sdk.ListToolsResult)
			stale := *original
			stale.Tools = append([]*sdk.Tool(nil), original.Tools...)
			material := *stale.Tools[0]
			material.Description = "obsolete observation"
			stale.Tools[0] = &material
			close(entered)
			select {
			case <-release:
				return &stale, nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
	})
	owner := newTestConnections(t)
	t.Cleanup(func() {
		unblock()
		if err := owner.shutdown(context.WithoutCancel(t.Context())); err != nil {
			t.Error(err)
		}
	})
	snapshots := catalogPublications(t, owner)
	if err := connectLaunch(owner, t.Context(), mustLaunch(t, config)); err != nil {
		t.Fatal(err)
	}
	initial := awaitCatalog(t, snapshots, func(tools []Executable) bool { return len(tools) == 1 })
	notice := &sdk.ToolListChangedRequest{Session: initial[0].session}
	owner.toolListChanged(t.Context(), notice)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("refresh never reached the controlled server")
	}
	for range 100 {
		owner.toolListChanged(t.Context(), notice)
	}
	if initial[0].Current() {
		t.Fatal("known invalid catalog remained dispatchable during refresh")
	}
	unblock()
	latest := awaitCatalog(t, snapshots, func(tools []Executable) bool {
		for _, candidate := range tools {
			if candidate.Definition().Description == "obsolete observation" {
				t.Fatal("stale refresh was published after a later notification")
			}
		}
		return len(tools) == 1 && tools[0].Current()
	})
	if queries.Load() != 3 || !latest[0].Current() {
		t.Fatalf("notifications were not coalesced: queries=%d, current=%t", queries.Load(), latest[0].Current())
	}
}

func TestLateToolNotificationCannotInvalidateAReplacementSession(t *testing.T) {
	_, config := catalogRemote(t, "files", "read")
	owner := newTestConnections(t)
	t.Cleanup(func() {
		if err := owner.shutdown(context.WithoutCancel(t.Context())); err != nil {
			t.Error(err)
		}
	})
	snapshots := catalogPublications(t, owner)
	if err := connectLaunch(owner, t.Context(), mustLaunch(t, config)); err != nil {
		t.Fatal(err)
	}
	initial := awaitCatalog(t, snapshots, func(tools []Executable) bool { return len(tools) == 1 })
	if err := connectLaunch(owner, t.Context(), mustLaunch(t, config)); err != nil {
		t.Fatal(err)
	}
	replacement := awaitCatalog(t, snapshots, func(tools []Executable) bool { return len(tools) == 1 && tools[0].session != initial[0].session })
	owner.toolListChanged(t.Context(), &sdk.ToolListChangedRequest{Session: initial[0].session})
	if !replacement[0].Current() || initial[0].Current() {
		t.Fatal("late notification moved the replacement session's catalog version")
	}
}
