package mcp

import (
	"context"
	"encoding/json/jsontext"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"testing/synctest"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func observedTestSession(t *testing.T, owner *connections, name mcpserver.ID, closed *atomic.Int32) (*sdk.ClientSession, *sdk.ServerSession, []Executable) {
	t.Helper()
	remote := sdk.NewServer(&sdk.Implementation{Name: "observed"}, nil)
	remote.AddTool(&sdk.Tool{Name: "read", InputSchema: jsontext.Value(`{"type":"object"}`)}, func(context.Context, *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
		return &sdk.CallToolResult{}, nil
	})
	serverTransport, clientTransport := sdk.NewInMemoryTransports()
	serverSession, err := remote.Connect(t.Context(), serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })
	clientSession, err := owner.client.Connect(t.Context(), clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	owner.mu.Lock()
	err = owner.ownSessionLocked(clientSession, func() error { closed.Add(1); return nil }, nil)
	owner.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	config := ServerConfig{Server: mcpserver.Server{Source: mcpserver.UserSource(), Name: name.Name()}}
	tools, err := sourceTools(t.Context(), owner, config, clientSession)
	if err != nil {
		t.Fatal(err)
	}
	owner.mu.Lock()
	current := owner.find(name)
	if current == nil {
		current = &server{id: name, config: config}
		owner.servers = append(owner.servers, current)
	}
	current.settle(clientSession, tools, mcpserver.ConnectionConnected, "", nil)
	owner.publishToolsLocked(name)
	owner.mu.Unlock()
	return clientSession, serverSession, tools
}

func TestNaturalSessionExitWithdrawsItsCatalogAndDispatchAuthority(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		owner := newTestConnections(t)
		var closes atomic.Int32
		session, remote, tools := observedTestSession(t, owner, testsupport.UserMCPServer("files"), &closes)
		var published atomic.Int32
		owner.setToolSink(func(tools []Executable, _, _ []mcpserver.ID) { published.Store(int32(len(tools))) })
		if !tools[0].Current() || published.Load() != 1 {
			t.Fatal("control session was not live")
		}
		if err := remote.Close(); err != nil {
			t.Fatal(err)
		}
		if err := session.Wait(); err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		statuses := owner.statuses()
		if len(statuses) != 1 || statuses[0].State != mcpserver.ConnectionFailed || statuses[0].Failure != mcpserver.FailureConnection || statuses[0].ToolCount != 0 {
			t.Fatalf("known terminal session remains live: %+v", statuses)
		}
		owner.mu.Lock()
		failure := owner.find(testsupport.UserMCPServer("files")).cause
		owner.mu.Unlock()
		if !errors.Is(failure, errSessionEnded) {
			t.Fatalf("terminal evidence was erased: %v", failure)
		}
		if published.Load() != 0 || tools[0].Current() {
			t.Fatal("terminal session retained advertised tools or dispatch authority")
		}
		if borrowed, err := tools[0].BorrowSession(t.Context()); borrowed != nil || !errors.Is(err, ErrUnavailable) {
			t.Fatalf("terminal session borrow = %v, %v", borrowed, err)
		}
		if err := owner.shutdown(t.Context()); err != nil {
			t.Fatal(err)
		}
		if closes.Load() != 1 {
			t.Fatalf("terminal session cleanup calls = %d, want 1", closes.Load())
		}
	})
}

func TestLateSessionExitCannotWithdrawItsReplacement(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		owner := newTestConnections(t)
		name := testsupport.UserMCPServer("files")
		var oldCloses, newCloses atomic.Int32
		_, oldRemote, oldTools := observedTestSession(t, owner, name, &oldCloses)
		_, _, newTools := observedTestSession(t, owner, name, &newCloses)
		if err := oldRemote.Close(); err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		statuses := owner.statuses()
		if len(statuses) != 1 || statuses[0].State != mcpserver.ConnectionConnected || statuses[0].ToolCount != 1 {
			t.Fatalf("late predecessor exit withdrew replacement: %+v", statuses)
		}
		if oldTools[0].Current() || !newTools[0].Current() {
			t.Fatal("dispatch authority did not follow exact session generation")
		}
		if err := owner.shutdown(t.Context()); err != nil {
			t.Fatal(err)
		}
		if oldCloses.Load() != 1 || newCloses.Load() != 1 {
			t.Fatalf("cleanup calls = %d, %d", oldCloses.Load(), newCloses.Load())
		}
	})
}

func TestSessionExitDetachAndShutdownShareOneRetirement(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		owner := newTestConnections(t)
		name := testsupport.UserMCPServer("files")
		var closes atomic.Int32
		_, remote, _ := observedTestSession(t, owner, name, &closes)
		stopped := make(chan error, 3)
		go func() { stopped <- remote.Close() }()
		go func() {
			err := owner.detach(name)
			if errors.Is(err, errConnectionsClosed) {
				err = nil
			}
			stopped <- err
		}()
		go func() { stopped <- owner.shutdown(t.Context()) }()
		for range 3 {
			if err := <-stopped; err != nil {
				t.Fatal(err)
			}
		}
		synctest.Wait()
		if closes.Load() != 1 || ownedSessionCount(owner) != 0 {
			t.Fatalf("session retirement was not joined exactly once: closes=%d, owned=%d", closes.Load(), ownedSessionCount(owner))
		}
	})
}

func TestSessionEndedDuringVerificationRetainsTerminalEvidence(t *testing.T) {
	owner := newTestConnections(t)
	t.Cleanup(func() {
		if err := owner.shutdown(context.WithoutCancel(t.Context())); err != nil {
			t.Error(err)
		}
	})
	remote := sdk.NewServer(&sdk.Implementation{Name: "verification"}, nil)
	remote.AddTool(&sdk.Tool{Name: "read", InputSchema: jsontext.Value(`{"type":"object"}`)}, func(context.Context, *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
		return &sdk.CallToolResult{}, nil
	})
	transport := httptest.NewServer(sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return remote }, nil))
	t.Cleanup(transport.Close)
	config := ServerConfig{Server: mcpserver.Server{Source: mcpserver.UserSource(), Name: testsupport.ServerName("files"), Transport: mcpserver.TransportStreamableHTTP, URL: transport.URL}}
	permit, err := newLaunch(config, nil, nil, admission{Run: func(ctx context.Context, session *sdk.ClientSession) error {
		if _, err := session.ListTools(ctx, nil); err != nil {
			return err
		}
		owner.mu.Lock()
		ended := owner.sessions[session].end.done
		owner.mu.Unlock()
		if err := session.Close(); err != nil {
			return err
		}
		<-ended
		return nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	err = connectLaunch(owner, t.Context(), permit)
	if !errors.Is(err, errSessionEnded) {
		t.Fatalf("terminal verified session publication = %v", err)
	}
	status := owner.statuses()
	if len(status) != 1 || status[0].State != mcpserver.ConnectionFailed || status[0].Failure != mcpserver.FailureConnection || status[0].ToolCount != 0 {
		t.Fatalf("ended session published a catalog: %+v", status)
	}
}
