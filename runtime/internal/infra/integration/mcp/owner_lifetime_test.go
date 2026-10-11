package mcp

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func TestConnectionOwnerRejectsAdmissionAfterLifetimeEnds(t *testing.T) {
	for _, entry := range attemptEntries() {
		t.Run(entry.name, func(t *testing.T) {
			lifetime, stop := context.WithCancelCause(t.Context())
			cause := errors.New("connection pool stopped")
			stop(cause)
			c := &Connections{lifetime: lifetime}
			attempt, err := entry.begin(c, t.Context(), testsupport.UserMCPServer("server"), func(context.Context, mcpserver.ID) (*Launch, error) {
				t.Error("stopped pool read configuration")
				return nil, errors.New("unexpected configuration read")
			})
			if attempt != nil || !errors.Is(err, context.Canceled) || !errors.Is(err, cause) || len(c.Statuses()) != 0 {
				t.Errorf("admission = (%v, %v), statuses = %+v; want rejected lifetime without mutation", attempt != nil, err, c.Statuses())
			}
			if err := c.Shutdown(t.Context()); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestConnectionOwnerCancelsUnstartedAttempts(t *testing.T) {
	for _, entry := range attemptEntries() {
		t.Run(entry.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				lifetime, stop := context.WithCancelCause(t.Context())
				defer stop(nil)
				cause := errors.New("connection pool stopped")
				c := &Connections{lifetime: lifetime}
				attempt, err := entry.begin(c, t.Context(), testsupport.UserMCPServer("server"), func(context.Context, mcpserver.ID) (*Launch, error) {
					t.Error("unstarted attempt read configuration after its owner stopped")
					return nil, errors.New("unexpected configuration read")
				})
				if err != nil {
					t.Fatal(err)
				}
				stop(cause)
				synctest.Wait()
				if err := attempt.Run(); !errors.Is(err, context.Canceled) || !errors.Is(err, cause) {
					t.Errorf("attempt = %v, want owner cancellation and cause", err)
				}
				if err := c.Shutdown(t.Context()); err != nil {
					t.Fatal(err)
				}
			})
		})
	}
}

func TestConnectionOwnerCancellationAbortsPreparationIO(t *testing.T) {
	for _, phase := range []string{"connect configuration", "authorize configuration", "credential restore"} {
		t.Run(phase, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				lifetime, stop := context.WithCancelCause(t.Context())
				defer stop(nil)
				cause := errors.New("connection pool stopped")
				readFailure := errors.New("preparation read failed")
				entered := make(chan struct{})
				forceExit := make(chan struct{})
				read := func(ctx context.Context) error {
					close(entered)
					select {
					case <-ctx.Done():
					case <-forceExit:
					}
					return readFailure
				}
				c := &Connections{lifetime: lifetime}
				config := ServerConfig{Server: mcpserver.Server{Source: mcpserver.UserSource(), Name: testsupport.ServerName("server"), Transport: mcpserver.TransportStreamableHTTP, URL: "https://example.invalid/mcp"}}
				resolve := func(ctx context.Context, _ mcpserver.ID) (*Launch, error) { return nil, read(ctx) }
				if phase == "credential restore" {
					c.oauthSessions = &oauthLoadStore{load: func(ctx context.Context, _ mcpserver.OAuthTarget) ([]byte, string, bool, error) {
						return nil, "", false, read(ctx)
					}}
					resolve = func(context.Context, mcpserver.ID) (*Launch, error) { return NewLaunch(config, nil, nil) }
				}
				begin := (*Connections).BeginConnect
				if phase == "authorize configuration" {
					begin = (*Connections).BeginAuthorize
				}
				attempt, err := begin(c, t.Context(), config.ID(), resolve)
				if err != nil {
					t.Fatal(err)
				}
				done := make(chan error, 1)
				go func() { done <- attempt.Run() }()
				<-entered
				stop(cause)
				synctest.Wait()
				select {
				case err = <-done:
				default:
					t.Error("owner cancellation did not abort preparation I/O")
					close(forceExit)
					err = <-done
				}
				if !errors.Is(err, readFailure) || !errors.Is(err, context.Canceled) || !errors.Is(err, cause) {
					t.Errorf("preparation = %v, want read failure, owner cancellation and cause", err)
				}
				if err := c.Shutdown(t.Context()); err != nil {
					t.Fatal(err)
				}
			})
		})
	}
}

func TestConnectionOwnerRejectsExpiredPreparationBeforeCredentialIO(t *testing.T) {
	lifetime, stop := context.WithCancelCause(t.Context())
	defer stop(nil)
	cause := errors.New("connection pool stopped")
	var reads, retired atomic.Int32
	c := &Connections{lifetime: lifetime, oauthSessions: &oauthLoadStore{load: func(context.Context, mcpserver.OAuthTarget) ([]byte, string, bool, error) {
		reads.Add(1)
		return nil, "", false, errors.New("unexpected credential read")
	}}}
	config := ServerConfig{Server: mcpserver.Server{Source: mcpserver.UserSource(), Name: testsupport.ServerName("server"), Transport: mcpserver.TransportStreamableHTTP, URL: "https://example.invalid/mcp"}}
	attempt, err := c.BeginConnect(t.Context(), config.ID(), func(context.Context, mcpserver.ID) (*Launch, error) {
		stop(cause)
		return NewLaunch(config, nil, func() error { retired.Add(1); return nil })
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := attempt.Run(); !errors.Is(err, context.Canceled) || !errors.Is(err, cause) {
		t.Errorf("preparation = %v, want owner cancellation and cause", err)
	}
	if reads.Load() != 0 || retired.Load() != 1 {
		t.Errorf("credential reads = %d, retirements = %d; want no I/O and settled resource", reads.Load(), retired.Load())
	}
	if err := c.Shutdown(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestConnectionOwnerRejectsRefusalAfterLifetimeEnds(t *testing.T) {
	lifetime, stop := context.WithCancelCause(t.Context())
	cause := errors.New("connection pool stopped")
	stop(cause)
	c := &Connections{lifetime: lifetime}
	err := c.Refuse(t.Context(), testsupport.UserMCPServer("server"), mcpserver.FailureConfiguration)
	if !errors.Is(err, context.Canceled) || !errors.Is(err, cause) || len(c.Statuses()) != 0 {
		t.Errorf("refusal = %v, statuses = %+v; want rejected lifetime without mutation", err, c.Statuses())
	}
	if err := c.Shutdown(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestConnectionOwnerCancellationRetainsPreparationPanic(t *testing.T) {
	for _, entry := range attemptEntries() {
		t.Run(entry.name, func(t *testing.T) {
			lifetime, stop := context.WithCancelCause(t.Context())
			defer stop(nil)
			cause := errors.New("connection pool stopped")
			failure := errors.New("configuration source panicked")
			c := &Connections{lifetime: lifetime}
			attempt, err := entry.begin(c, t.Context(), testsupport.UserMCPServer("server"), func(context.Context, mcpserver.ID) (*Launch, error) {
				stop(cause)
				panic(failure)
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := attempt.Run(); err == nil || !strings.Contains(err.Error(), failure.Error()) || !errors.Is(err, context.Canceled) || !errors.Is(err, cause) {
				t.Errorf("preparation panic = %v, want panic, owner cancellation and cause", err)
			}
			if err := c.Shutdown(t.Context()); err != nil {
				t.Fatal(err)
			}
		})
	}
}
