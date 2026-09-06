package mcp

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
)

func TestAuthorizationAttemptReportsFailureWithoutDiscardingResult(t *testing.T) {
	ports := &fakePorts{
		statuses:     []mcpserver.ConnectionStatus{{Name: testMCPServerName("github")}},
		authorizeErr: errors.New("oauth exchange exposed a secret-bearing response"),
	}
	c := testCoordinator(t, configWithPorts(ports))
	defer requireCoordinatorShutdown(t, c)

	created, err := c.CreateAuthorizationAttempt(context.Background(), testMCPServerName("github"))
	if err != nil {
		t.Fatalf("CreateAuthorizationAttempt: %v", err)
	}
	settled := awaitAuthorizationAttempt(t, c, created.ID)
	if settled.Status != AuthorizationAttemptFailed || settled.FinishedAt == nil {
		t.Fatalf("settled attempt = %+v, want failed", settled)
	}
}

func TestAuthorizationAttemptIsCanceledWhenSuperseded(t *testing.T) {
	authorizeStarted := make(chan string, 1)
	ports := &fakePorts{
		statuses:         []mcpserver.ConnectionStatus{{Name: testMCPServerName("github"), State: mcpserver.ConnectionConnected}},
		authorizeStarted: authorizeStarted,
		releaseAuthorize: make(chan struct{}),
	}
	c := testCoordinator(t, configWithPorts(ports))
	defer requireCoordinatorShutdown(t, c)

	created, err := c.CreateAuthorizationAttempt(context.Background(), testMCPServerName("github"))
	if err != nil {
		t.Fatalf("CreateAuthorizationAttempt: %v", err)
	}
	<-authorizeStarted
	if err := c.ReconnectServer(context.Background(), testMCPServerName("github")); err != nil {
		t.Fatalf("ReconnectServer: %v", err)
	}
	settled := awaitAuthorizationAttempt(t, c, created.ID)
	if settled.Status != AuthorizationAttemptCanceled || settled.FinishedAt == nil {
		t.Fatalf("settled attempt = %+v, want canceled", settled)
	}
}

func TestAuthorizationAttemptIsCanceledWhenSupersededDuringRegistryRead(t *testing.T) {
	name := testMCPServerName("github")
	ports := &fakePorts{
		statuses:         []mcpserver.ConnectionStatus{{Name: name, State: mcpserver.ConnectionConnected}},
		authorizeStarted: make(chan string, 1),
		releaseAuthorize: make(chan struct{}),
	}
	cfg := configWithPorts(ports)
	registry := &cancelableRegistryRead{
		Registry: cfg.Registry,
		block:    make(chan struct{}, 1),
		started:  make(chan struct{}),
	}
	cfg.Registry = registry
	c := testCoordinator(t, cfg)
	created, err := c.CreateAuthorizationAttempt(t.Context(), name)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-ports.authorizeStarted:
	case <-time.After(time.Second):
		t.Fatal("authorization did not start")
	}
	registry.block <- struct{}{}
	close(ports.releaseAuthorize)
	select {
	case <-registry.started:
	case <-time.After(time.Second):
		t.Fatal("authorization did not reach the registry read")
	}
	if err := c.ReconnectServer(t.Context(), name); err != nil {
		t.Fatal(err)
	}
	settled := awaitAuthorizationAttempt(t, c, created.ID)
	if settled.Status != AuthorizationAttemptCanceled || settled.FinishedAt == nil {
		t.Fatalf("superseded attempt = %+v, want canceled", settled)
	}
}

type cancelableRegistryRead struct {
	Registry
	block   chan struct{}
	started chan struct{}
}

func (r *cancelableRegistryRead) Get(ctx context.Context, name mcpserver.ServerName) (mcpserver.Server, bool, error) {
	select {
	case <-r.block:
		close(r.started)
		<-ctx.Done()
		return mcpserver.Server{}, false, ctx.Err()
	default:
		return r.Registry.Get(ctx, name)
	}
}

func TestAuthorizationAttemptStoreRetainsOnlyTerminalResults(t *testing.T) {
	now := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	attemptID, err := ParseAuthorizationAttemptID(testAuthorizationAttemptID)
	if err != nil {
		t.Fatalf("parse test authorization attempt identity: %v", err)
	}
	store := newAuthorizationAttemptStoreWith(
		func() time.Time { return now },
		func() AuthorizationAttemptID { return attemptID },
		time.Minute,
	)

	pending := store.create(testMCPServerName("github"))
	now = now.Add(2 * time.Minute)
	if _, ok := store.get(pending.ID); !ok {
		t.Fatal("pending attempt expired")
	}
	store.settle(pending.ID, AuthorizationAttemptSucceeded)
	now = now.Add(time.Minute - time.Nanosecond)
	if _, ok := store.get(pending.ID); !ok {
		t.Fatal("terminal attempt expired before retention elapsed")
	}
	now = now.Add(time.Nanosecond)
	if _, ok := store.get(pending.ID); ok {
		t.Fatal("terminal attempt survived its retention window")
	}
}

func TestAuthorizationAttemptRejectsUnknownID(t *testing.T) {
	c := testCoordinator(t, Config{})
	if _, err := c.AuthorizationAttempt(context.Background(), testAuthorizationAttemptID); !errors.Is(err, ErrAuthorizationAttemptNotFound) {
		t.Fatalf("AuthorizationAttempt = %v, want ErrAuthorizationAttemptNotFound", err)
	}
	if _, err := c.AuthorizationAttempt(context.Background(), "mcpauth_missing"); !errors.Is(err, ErrAuthorizationAttemptNotFound) {
		t.Fatalf("malformed AuthorizationAttempt = %v, want ErrAuthorizationAttemptNotFound", err)
	}
}

func TestAuthorizationAttemptRejectsNonHTTPServerBeforeDispatch(t *testing.T) {
	name := testMCPServerName("filesystem")
	ports := &fakePorts{statuses: []mcpserver.ConnectionStatus{{Name: name}}}
	registry := &testRegistry{servers: map[mcpserver.ServerName]mcpserver.Server{
		name: {
			Name: name, Enabled: true,
			Transport: mcpserver.TransportStdio, Command: "mcp-filesystem",
		},
	}}
	c := testCoordinator(t, Config{
		Registry: registry, StatusReader: ports,
		ConnectionControl: ports,
	})

	if _, err := c.CreateAuthorizationAttempt(context.Background(), name); !errors.Is(err, ErrAuthorizationUnsupported) {
		t.Fatalf("CreateAuthorizationAttempt = %v, want ErrAuthorizationUnsupported", err)
	}
	if ports.authorizeName != "" {
		t.Fatalf("unsupported server dispatched authorization as %q", ports.authorizeName)
	}
}
