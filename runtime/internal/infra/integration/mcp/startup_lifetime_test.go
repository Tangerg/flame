package mcp

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	sdkmcp "github.com/Tangerg/go-sdk/mcp"
)

func TestStartupRejectsCanceledAdmission(t *testing.T) {
	for _, owner := range []string{"request", "session owner"} {
		for _, catalog := range []string{"empty", "configured"} {
			t.Run(owner+"/"+catalog, func(t *testing.T) {
				ctx, lifetime, cancel := startupLifetime(t, owner)
				cause := errors.New("initialization stopped")
				cancel(cause)
				var configs []ServerConfig
				if catalog == "configured" {
					configs = []ServerConfig{startupDescriptor("unstarted")}
				}
				connections, err := Open(ctx, lifetime, configs, nil, func(context.Context, mcpserver.ID) (*Launch, error) {
					t.Error("canceled initialization read configuration")
					return nil, errors.New("unexpected configuration read")
				}, nil, NewClient())
				if connections != nil {
					defer connections.Shutdown(context.WithoutCancel(t.Context()))
				}
				if connections != nil || !errors.Is(err, context.Canceled) || !errors.Is(err, cause) {
					t.Fatalf("Open = (%v, %v), want rejected initialization with cancellation and cause", connections != nil, err)
				}
			})
		}
	}
}

func TestStartupHandoffDoesNotBorrowCallerLifetime(t *testing.T) {
	caller, cancelCaller := context.WithCancel(t.Context())
	lifetime, cancelOwner := context.WithCancel(t.Context())
	defer cancelOwner()
	entered := make(chan struct{})
	settled := make(chan struct{})
	c, err := Open(caller, lifetime, []ServerConfig{startupDescriptor("blocked")}, nil, func(ctx context.Context, _ mcpserver.ID) (*Launch, error) {
		close(entered)
		<-ctx.Done()
		close(settled)
		return nil, ctx.Err()
	}, nil, NewClient())
	if err != nil {
		t.Fatal(err)
	}
	<-entered
	cancelCaller()
	select {
	case <-settled:
		t.Fatal("construction caller retired an accepted owner task")
	default:
	}
	if status := c.Statuses(); len(status) != 1 || status[0].State != mcpserver.ConnectionConnecting || status[0].ToolCount != 0 {
		t.Fatalf("pending source became executable: %+v", status)
	}
	if err := c.Shutdown(context.WithoutCancel(t.Context())); err != nil {
		t.Fatal(err)
	}
	select {
	case <-settled:
	default:
		t.Fatal("shutdown returned before admitted configuration settled")
	}
}

func TestStartupShutdownRetainsSessionRetirementDiagnostics(t *testing.T) {
	remote := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "startup-test"}, nil)
	addRemoteTool(t, remote, "read")
	transport := httptest.NewServer(sdkmcp.NewStreamableHTTPHandler(func(*http.Request) *sdkmcp.Server { return remote }, nil))
	defer transport.Close()
	cfg := ServerConfig{Server: mcpserver.Server{Source: mcpserver.UserSource(), Name: testsupport.ServerName("healthy"), Transport: mcpserver.TransportStreamableHTTP, URL: transport.URL}}
	failure := errors.New("launch resource retirement failed")
	var retired atomic.Int32
	c, err := Open(t.Context(), t.Context(), []ServerConfig{cfg}, nil, func(context.Context, mcpserver.ID) (*Launch, error) {
		return NewLaunch(cfg, nil, func() error { retired.Add(1); return failure }, Admission{})
	}, nil, NewClient())
	if err != nil {
		t.Fatal(err)
	}
	awaitInitialConnections(c)
	if err := c.Shutdown(context.WithoutCancel(t.Context())); !errors.Is(err, failure) {
		t.Fatalf("shutdown lost retirement cause: %v", err)
	}
	if err := c.Shutdown(context.WithoutCancel(t.Context())); !errors.Is(err, failure) || retired.Load() != 1 {
		t.Fatalf("retirement repeated or lost its diagnostic: %d, %v", retired.Load(), err)
	}
}

func TestStartupToleratesIndependentHandshakeTimeout(t *testing.T) {
	remote := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "startup-test", Version: "v1"}, nil)
	addRemoteTool(t, remote, "read")
	handler := sdkmcp.NewStreamableHTTPHandler(func(*http.Request) *sdkmcp.Server { return remote }, nil)
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/blocked" {
			_, _ = io.Copy(io.Discard, r.Body)
			<-r.Context().Done()
			return
		}
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(httpServer.Close)
	timedOut := ServerConfig{Server: mcpserver.Server{
		Source: mcpserver.UserSource(), Name: testsupport.ServerName("timed-out"),
		Transport: mcpserver.TransportStreamableHTTP, URL: httpServer.URL + "/blocked",
		HandshakeTimeout: mustHandshakeTimeout(t, time.Second),
	}}
	healthy := ServerConfig{Server: mcpserver.Server{Source: mcpserver.UserSource(), Name: testsupport.ServerName("healthy"), Transport: mcpserver.TransportStreamableHTTP, URL: httpServer.URL}}
	connections, err := Open(t.Context(), t.Context(), []ServerConfig{timedOut, healthy}, nil, func(_ context.Context, name mcpserver.ID) (*Launch, error) {
		if name == timedOut.ID() {
			return NewLaunch(timedOut, nil, nil, Admission{})
		}
		return NewLaunch(healthy, nil, nil, Admission{})
	}, nil, NewClient())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := connections.Shutdown(context.WithoutCancel(t.Context())); err != nil {
			t.Error(err)
		}
	})
	awaitInitialConnections(connections)
	statuses := connections.Statuses()
	if len(statuses) != 2 || statuses[0].State != mcpserver.ConnectionFailed || statuses[1].State != mcpserver.ConnectionConnected {
		t.Fatalf("startup statuses = %+v, want tolerated timeout and healthy connection", statuses)
	}
	if tools := liveToolNames(connections, healthy.ID()); len(tools) != 1 || tools[0] != "healthy_read" {
		t.Fatalf("healthy tools = %v, want healthy_read", tools)
	}
}

func startupLifetime(t *testing.T, owner string) (context.Context, context.Context, context.CancelCauseFunc) {
	t.Helper()
	ctx, cancel := context.WithCancelCause(t.Context())
	t.Cleanup(func() { cancel(nil) })
	if owner == "request" {
		return ctx, t.Context(), cancel
	}
	return t.Context(), ctx, cancel
}

func startupDescriptor(name string) ServerConfig {
	return ServerConfig{Server: mcpserver.Server{Source: mcpserver.UserSource(), Name: testsupport.ServerName(name), Transport: mcpserver.TransportStdio, Command: "must-not-start"}}
}
