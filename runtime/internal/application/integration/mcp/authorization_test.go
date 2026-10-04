package mcp

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
)

func TestAuthorizationReadFailureRetiresConnectingStatus(t *testing.T) {
	name := testMCPServerName("github")
	ports := &fakePorts{
		statuses:         []mcpserver.ConnectionStatus{{Name: name, State: mcpserver.ConnectionConnected}},
		authorizeStarted: make(chan string, 1),
		releaseAuthorize: make(chan struct{}),
	}
	cfg := configWithPorts(ports)
	registry := &failingConnectionRead{Registry: cfg.Registry}
	cfg.Registry = registry
	states := make(chan mcpserver.ConnectionState, 4)
	var c *Coordinator
	cfg.Invalidations = func(invalidation.Notice) {
		states <- testServerStatus(c, name).State
	}
	c = testCoordinator(t, cfg)
	defer requireCoordinatorShutdown(t, c)
	created, err := c.CreateAuthorizationAttempt(t.Context(), name)
	if err != nil {
		t.Fatal(err)
	}
	<-ports.authorizeStarted
	registry.failed.Store(true)
	close(ports.releaseAuthorize)
	settled := awaitAuthorizationAttempt(t, c, created.ID)
	if settled.Status != AuthorizationAttemptFailed {
		t.Fatalf("authorization attempt = %s, want failed", settled.Status)
	}
	if status := testServerStatus(c, name); status.State != mcpserver.ConnectionConnected {
		t.Fatalf("retired authorization status = %+v, want live connected state", status)
	}
	for _, want := range []mcpserver.ConnectionState{mcpserver.ConnectionConnecting, mcpserver.ConnectionConnected} {
		select {
		case got := <-states:
			if got != want {
				t.Fatalf("authorization invalidation state = %s, want %s", got, want)
			}
		case <-time.After(time.Second):
			t.Fatal("authorization retirement did not publish its live state")
		}
	}
}

func TestAuthorizationShutdownRetiresConnectingStatus(t *testing.T) {
	name := testMCPServerName("github")
	ports := &fakePorts{
		statuses:         []mcpserver.ConnectionStatus{{Name: name, State: mcpserver.ConnectionConnected}},
		authorizeStarted: make(chan string, 1),
		releaseAuthorize: make(chan struct{}),
	}
	c := testCoordinator(t, configWithPorts(ports))
	created, err := c.CreateAuthorizationAttempt(t.Context(), name)
	if err != nil {
		t.Fatal(err)
	}
	<-ports.authorizeStarted
	requireCoordinatorShutdown(t, c)
	settled, err := c.AuthorizationAttempt(t.Context(), created.ID.String())
	if err != nil || settled.Status != AuthorizationAttemptCanceled {
		t.Fatalf("authorization after shutdown = %+v, %v", settled, err)
	}
	if status := testServerStatus(c, name); status.State != mcpserver.ConnectionConnected {
		t.Fatalf("shutdown status = %+v, want live connected state", status)
	}
}

func TestQueuedConnectingNotificationCannotReviveRetiredAuthorization(t *testing.T) {
	name := testMCPServerName("github")
	ports := &fakePorts{
		statuses:         []mcpserver.ConnectionStatus{{Name: name, State: mcpserver.ConnectionConnected}},
		authorizeStarted: make(chan string, 1),
		releaseAuthorize: make(chan struct{}),
	}
	cfg := configWithPorts(ports)
	registry := &failingConnectionRead{Registry: cfg.Registry}
	cfg.Registry = registry
	firstNotice := make(chan struct{})
	releaseNotice := make(chan struct{})
	var blocked atomic.Bool
	cfg.Invalidations = func(invalidation.Notice) {
		if blocked.CompareAndSwap(false, true) {
			close(firstNotice)
			<-releaseNotice
		}
	}
	c := testCoordinator(t, cfg)
	defer requireCoordinatorShutdown(t, c)
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseNotice) }) }
	defer release()
	if err := c.ReconnectServer(t.Context(), name); err != nil {
		t.Fatal(err)
	}
	select {
	case <-firstNotice:
	case <-time.After(time.Second):
		t.Fatal("reconnect did not publish its initial state")
	}
	created, err := c.CreateAuthorizationAttempt(t.Context(), name)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-ports.authorizeStarted:
	case <-time.After(time.Second):
		t.Fatal("authorization waited for an earlier notification")
	}
	registry.failed.Store(true)
	close(ports.releaseAuthorize)
	if settled := awaitAuthorizationAttempt(t, c, created.ID); settled.Status != AuthorizationAttemptFailed {
		t.Fatalf("authorization attempt = %s, want failed", settled.Status)
	}
	release()
	requireCoordinatorShutdown(t, c)
	if status := testServerStatus(c, name); status.State != mcpserver.ConnectionConnected {
		t.Fatalf("queued notification revived a retired dial: %+v", status)
	}
}

func TestQueuedRegistryNotificationCannotHideCompletedConnection(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		name := testMCPServerName("github")
		ports := &fakePorts{statuses: []mcpserver.ConnectionStatus{{Name: name, State: mcpserver.ConnectionConnected}}}
		cfg := configWithPorts(ports)
		firstNotice := make(chan struct{})
		releaseNotice := make(chan struct{})
		var blocked atomic.Bool
		cfg.Invalidations = func(invalidation.Notice) {
			if blocked.CompareAndSwap(false, true) {
				close(firstNotice)
				<-releaseNotice
			}
		}
		c := testCoordinator(t, cfg)
		var releaseOnce sync.Once
		release := func() { releaseOnce.Do(func() { close(releaseNotice) }) }
		defer requireCoordinatorShutdown(t, c)
		defer release()
		if err := c.ReconnectServer(t.Context(), name); err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		select {
		case <-firstNotice:
		default:
			t.Fatal("reconnect did not publish its initial state")
		}
		for _, enabled := range []bool{false, true} {
			if _, err := c.UpdateServer(t.Context(), name, ServerPatch{Enabled: &enabled}); err != nil {
				t.Fatal(err)
			}
		}
		synctest.Wait()
		release()
		synctest.Wait()
		if status := testServerStatus(c, name); !status.Known || status.State != mcpserver.ConnectionConnected {
			t.Fatalf("queued registry notification hid the completed connection: %+v", status)
		}
	})
}

type failingConnectionRead struct {
	Registry
	failed atomic.Bool
}

func (r *failingConnectionRead) Definition(ctx context.Context, name mcpserver.ServerName) (mcpserver.Server, bool, error) {
	if r.failed.Load() {
		return mcpserver.Server{}, false, errors.New("registry read failed")
	}
	return r.Registry.Definition(ctx, name)
}

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

func (r *cancelableRegistryRead) Definition(ctx context.Context, name mcpserver.ServerName) (mcpserver.Server, bool, error) {
	select {
	case <-r.block:
		close(r.started)
		<-ctx.Done()
		return mcpserver.Server{}, false, ctx.Err()
	default:
		return r.Registry.Definition(ctx, name)
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
