package mcp

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"testing/synctest"
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
				connections, err := Dial(ctx, lifetime, configs, nil, func(context.Context, mcpserver.ID) (*Launch, error) {
					t.Error("canceled initialization read configuration")
					return nil, errors.New("unexpected configuration read")
				})
				if connections != nil {
					defer connections.Shutdown(context.WithoutCancel(t.Context()))
				}
				if connections != nil || !errors.Is(err, context.Canceled) || !errors.Is(err, cause) {
					t.Fatalf("Dial = (%v, %v), want rejected initialization with cancellation and cause", connections != nil, err)
				}
			})
		}
	}
}

func TestStartupCancellationAbortsConfiguration(t *testing.T) {
	for _, owner := range []string{"request", "session owner"} {
		t.Run(owner, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx, lifetime, cancel := startupLifetime(t, owner)
				cause := errors.New("initialization stopped")
				readFailure := errors.New("configuration read failed")
				entered := make(chan struct{})
				forceExit := make(chan struct{})
				type outcome struct {
					connections *Connections
					err         error
				}
				done := make(chan outcome, 1)
				go func() {
					connections, err := Dial(ctx, lifetime, []ServerConfig{startupDescriptor("blocked")}, nil, func(ctx context.Context, _ mcpserver.ID) (*Launch, error) {
						close(entered)
						select {
						case <-ctx.Done():
						case <-forceExit:
						}
						return nil, readFailure
					})
					done <- outcome{connections, err}
				}()
				<-entered
				cancel(cause)
				synctest.Wait()
				var result outcome
				select {
				case result = <-done:
				default:
					t.Error("initialization cancellation did not abort configuration")
					close(forceExit)
					result = <-done
				}
				if result.connections != nil {
					defer result.connections.Shutdown(context.WithoutCancel(t.Context()))
				}
				if result.connections != nil || !errors.Is(result.err, readFailure) || !errors.Is(result.err, context.Canceled) || !errors.Is(result.err, cause) {
					t.Fatalf("Dial = (%v, %v), want failed read, cancellation and cause", result.connections != nil, result.err)
				}
			})
		})
	}
}

func TestStartupCancellationSettlesAcquiredSessionsBeforeReturning(t *testing.T) {
	for _, owner := range []string{"request", "session owner"} {
		t.Run(owner, func(t *testing.T) {
			ctx, lifetime, cancel := startupLifetime(t, owner)
			cause := errors.New("initialization stopped")
			readFailure := errors.New("configuration read failed")
			closeFailure := errors.New("launch resource retirement failed")
			remote := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "startup-test", Version: "v1"}, nil)
			addRemoteTool(t, remote, "read")
			httpServer := httptest.NewServer(sdkmcp.NewStreamableHTTPHandler(func(*http.Request) *sdkmcp.Server { return remote }, nil))
			t.Cleanup(httpServer.Close)
			healthy := ServerConfig{Server: mcpserver.Server{Source: mcpserver.UserSource(), Name: testsupport.ServerName("healthy"), Transport: mcpserver.TransportStreamableHTTP, URL: httpServer.URL}}
			stopped := startupDescriptor("stopped")
			unstarted := startupDescriptor("unstarted")
			var retired, unexpectedReads atomic.Int32
			connections, err := Dial(ctx, lifetime, []ServerConfig{healthy, stopped, unstarted}, nil, func(_ context.Context, name mcpserver.ID) (*Launch, error) {
				switch name {
				case healthy.ID():
					return NewLaunch(healthy, nil, func() error { retired.Add(1); return closeFailure })
				case stopped.ID():
					cancel(cause)
					return nil, readFailure
				default:
					unexpectedReads.Add(1)
					return nil, errors.New("unexpected configuration read")
				}
			})
			if connections != nil {
				defer connections.Shutdown(context.WithoutCancel(t.Context()))
			}
			if connections != nil || !errors.Is(err, readFailure) || !errors.Is(err, closeFailure) || !errors.Is(err, context.Canceled) || !errors.Is(err, cause) {
				t.Errorf("Dial = (%v, %v), want failed initialization with read, retirement and cancellation causes", connections != nil, err)
			}
			if retired.Load() != 1 || unexpectedReads.Load() != 0 {
				t.Errorf("retirements = %d, later reads = %d; want settled resource and no later I/O", retired.Load(), unexpectedReads.Load())
			}
		})
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
	connections, err := Dial(t.Context(), t.Context(), []ServerConfig{timedOut, healthy}, nil, func(_ context.Context, name mcpserver.ID) (*Launch, error) {
		if name == timedOut.ID() {
			return NewLaunch(timedOut, nil, nil)
		}
		return NewLaunch(healthy, nil, nil)
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := connections.Shutdown(context.WithoutCancel(t.Context())); err != nil {
			t.Error(err)
		}
	})
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
