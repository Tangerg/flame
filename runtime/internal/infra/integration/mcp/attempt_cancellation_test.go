package mcp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"testing/synctest"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	sdkmcp "github.com/Tangerg/go-sdk/mcp"
)

type attemptEntry struct {
	name  string
	begin func(*Connections, context.Context, mcpserver.ID, func(context.Context, mcpserver.ID) (*Launch, error)) (*Attempt, error)
}

func attemptEntries() []attemptEntry {
	return []attemptEntry{
		{"connect", (*Connections).BeginConnect},
		{"authorize", (*Connections).BeginAuthorize},
	}
}

func TestAttemptCanceledAdmissionRetainsLifetimeBeforeMutation(t *testing.T) {
	for _, entry := range attemptEntries() {
		t.Run(entry.name, func(t *testing.T) {
			c := &Connections{lifetime: t.Context()}
			failure := errors.New("connection owner stopped")
			ctx, cancel := context.WithCancelCause(t.Context())
			cancel(failure)
			attempt, err := entry.begin(c, ctx, testsupport.UserMCPServer("server"), func(context.Context, mcpserver.ID) (*Launch, error) {
				t.Error("canceled admission resolved configuration")
				return nil, errors.New("unexpected configuration read")
			})
			if attempt != nil || !errors.Is(err, context.Canceled) || !errors.Is(err, failure) {
				t.Errorf("admission = (%v, %v), want cancellation and cause", attempt, err)
			}
			if len(c.Statuses()) != 0 {
				t.Error("canceled admission changed connection state")
			}
			if err := c.Shutdown(t.Context()); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestUnstartedAttemptRetainsCanceledLifetime(t *testing.T) {
	for _, entry := range attemptEntries() {
		t.Run(entry.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				c := &Connections{lifetime: t.Context()}
				failure := errors.New("connection owner stopped")
				ctx, cancel := context.WithCancelCause(t.Context())
				defer cancel(nil)
				attempt, err := entry.begin(c, ctx, testsupport.UserMCPServer("server"), func(context.Context, mcpserver.ID) (*Launch, error) {
					t.Error("unstarted canceled attempt resolved configuration")
					return nil, errors.New("unexpected configuration read")
				})
				if err != nil {
					t.Fatal(err)
				}
				cancel(failure)
				synctest.Wait()
				if err := attempt.Run(); !errors.Is(err, context.Canceled) || !errors.Is(err, failure) {
					t.Errorf("attempt = %v, want cancellation and cause", err)
				}
				if err := c.Shutdown(t.Context()); err != nil {
					t.Fatal(err)
				}
			})
		})
	}
}

func TestAttemptConfigurationFailureRetainsConcurrentCancellation(t *testing.T) {
	for _, entry := range attemptEntries() {
		t.Run(entry.name, func(t *testing.T) {
			c := &Connections{lifetime: t.Context()}
			failure := errors.New("configuration read failed")
			cause := errors.New("connection owner stopped")
			ctx, cancel := context.WithCancelCause(t.Context())
			defer cancel(nil)
			attempt, err := entry.begin(c, ctx, testsupport.UserMCPServer("server"), func(context.Context, mcpserver.ID) (*Launch, error) {
				cancel(cause)
				return nil, failure
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := attempt.Run(); !errors.Is(err, context.Canceled) || !errors.Is(err, cause) || !errors.Is(err, failure) {
				t.Errorf("attempt = %v, want read failure, cancellation and cause", err)
			}
			if err := c.Shutdown(t.Context()); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestAttemptRejectsCanceledConfigurationBeforeExternalIO(t *testing.T) {
	for _, entry := range attemptEntries() {
		t.Run(entry.name, func(t *testing.T) {
			c := &Connections{lifetime: t.Context()}
			cause := errors.New("connection owner stopped")
			ctx, cancel := context.WithCancelCause(t.Context())
			defer cancel(nil)
			config := ServerConfig{Server: mcpserver.Server{
				Source: mcpserver.UserSource(), Name: testsupport.ServerName("server"),
				Transport: mcpserver.TransportStreamableHTTP, URL: "https://example.invalid/mcp",
			}}
			var retired int
			input, err := NewLaunch(config, nil, func() error { retired++; return nil }, Admission{})
			if err != nil {
				t.Fatal(err)
			}
			attempt, err := entry.begin(c, ctx, config.ID(), func(context.Context, mcpserver.ID) (*Launch, error) {
				cancel(cause)
				return input, nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := attempt.Run(); !errors.Is(err, context.Canceled) || !errors.Is(err, cause) {
				t.Errorf("configuration admission = %v, want cancellation and cause", err)
			}
			if retired != 1 {
				t.Errorf("launch retirements = %d, want one", retired)
			}
			if err := c.Shutdown(t.Context()); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestAttemptRetirementWaitRetainsCanceledLifetimeAndOwnedCleanup(t *testing.T) {
	for _, entry := range attemptEntries() {
		t.Run(entry.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				name := testsupport.UserMCPServer("server")
				closeFailure := errors.New("retired session close failed")
				cause := errors.New("connection owner stopped")
				release := make(chan struct{})
				finish := sync.OnceFunc(func() { close(release) })
				defer finish()
				session := new(sdkmcp.ClientSession)
				c := &Connections{
					lifetime: t.Context(),
					servers:  []*server{{id: name, session: session, state: mcpserver.ConnectionConnected}},
					sessions: map[*sdkmcp.ClientSession]*ownedSession{session: {closeFn: func() error { <-release; return closeFailure }}},
				}
				ctx, cancel := context.WithCancelCause(t.Context())
				defer cancel(nil)
				attempt, err := entry.begin(c, ctx, name, func(context.Context, mcpserver.ID) (*Launch, error) {
					t.Error("canceled retirement waiter resolved configuration")
					return nil, errors.New("unexpected configuration read")
				})
				if err != nil {
					t.Fatal(err)
				}
				done := make(chan error, 1)
				go func() { done <- attempt.Run() }()
				synctest.Wait()
				cancel(cause)
				synctest.Wait()
				if err := <-done; !errors.Is(err, context.Canceled) || !errors.Is(err, cause) {
					t.Errorf("retirement wait = %v, want cancellation and cause", err)
				}
				finish()
				if err := c.Shutdown(t.Context()); !errors.Is(err, closeFailure) {
					t.Errorf("shutdown = %v, want owned retirement failure", err)
				}
			})
		})
	}
}

func TestRefusalRetainsCanceledLifetimeBeforeMutation(t *testing.T) {
	c := &Connections{lifetime: t.Context()}
	cause := errors.New("configuration owner stopped")
	ctx, cancel := context.WithCancelCause(t.Context())
	cancel(cause)
	if err := c.Refuse(ctx, testsupport.UserMCPServer("server"), mcpserver.FailureConfiguration); !errors.Is(err, context.Canceled) || !errors.Is(err, cause) {
		t.Errorf("refusal = %v, want cancellation and cause", err)
	}
	if len(c.Statuses()) != 0 {
		t.Error("canceled refusal changed connection state")
	}
}

func TestPublishedAttemptRetainsSuccessAfterCallerCancellation(t *testing.T) {
	remote := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "test-server", Version: "v1"}, nil)
	addRemoteTool(t, remote, "read")
	transport := httptest.NewServer(sdkmcp.NewStreamableHTTPHandler(func(*http.Request) *sdkmcp.Server { return remote }, nil))
	t.Cleanup(transport.Close)
	config := ServerConfig{Server: mcpserver.Server{
		Source: mcpserver.UserSource(), Name: testsupport.ServerName("server"),
		Transport: mcpserver.TransportStreamableHTTP, URL: transport.URL,
	}}
	c := &Connections{lifetime: t.Context(), client: NewClient()}
	t.Cleanup(func() {
		if err := c.Shutdown(context.WithoutCancel(t.Context())); err != nil {
			t.Error(err)
		}
	})
	var catalog []Executable
	c.SetToolSink(func(tools []Executable, _, _ []mcpserver.ID) { catalog = tools })
	ctx, cancel := context.WithCancelCause(t.Context())
	defer cancel(nil)
	attempt, err := c.BeginConnect(ctx, config.ID(), func(context.Context, mcpserver.ID) (*Launch, error) {
		return NewLaunch(config, nil, nil, Admission{})
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := attempt.Run(); err != nil {
		t.Fatal(err)
	}
	cancel(errors.New("connection waiter stopped"))
	if err := attempt.Run(); err != nil {
		t.Fatalf("published attempt = %v, want retained success", err)
	}
	statuses := c.Statuses()
	if len(statuses) != 1 || statuses[0].State != mcpserver.ConnectionConnected || len(catalog) != 1 || !catalog[0].Current() {
		t.Fatalf("caller cancellation retired a published session: statuses=%v tools=%v", statuses, toolNames(catalog))
	}
}
