package mcp

import (
	"context"
	"errors"
	"github.com/Tangerg/flame/packages/go-sdk/mcpconnector"
	"log/slog"
	"sync"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
)

const (
	authorizationAttemptRetention = mcpconnector.AttemptRetentionSeconds * time.Second
)

// AuthorizationAttemptStatus is the lifecycle of one interactive MCP OAuth
// flow. A terminal attempt remains readable for the retention published by
// [Coordinator.AuthorizationAttemptRetention] so clients can recover after a
// transient transport interruption.
type AuthorizationAttemptStatus string

const (
	AuthorizationAttemptUnconfirmed AuthorizationAttemptStatus = AuthorizationAttemptStatus(mcpconnector.Unconfirmed)
	AuthorizationAttemptPending     AuthorizationAttemptStatus = AuthorizationAttemptStatus(mcpconnector.Pending)
	AuthorizationAttemptSucceeded   AuthorizationAttemptStatus = AuthorizationAttemptStatus(mcpconnector.Succeeded)
	AuthorizationAttemptFailed      AuthorizationAttemptStatus = AuthorizationAttemptStatus(mcpconnector.Failed)
	AuthorizationAttemptCanceled    AuthorizationAttemptStatus = AuthorizationAttemptStatus(mcpconnector.Canceled)
)

func (a AuthorizationAttemptStatus) Valid() bool {
	return a == AuthorizationAttemptPending || a == AuthorizationAttemptSucceeded ||
		a == AuthorizationAttemptFailed || a == AuthorizationAttemptCanceled || a == AuthorizationAttemptUnconfirmed
}

// AuthorizationAttempt is the application read model for one interactive
// OAuth flow. Failure details remain in telemetry; callers receive only the
// stable status.
type AuthorizationAttempt struct {
	ID         AuthorizationAttemptID
	Server     mcpserver.ID
	Status     AuthorizationAttemptStatus
	CreatedAt  time.Time
	FinishedAt *time.Time
}

// AuthorizationAttemptRetention is the terminal-result window this
// Coordinator enforces and reports to callers.
func (c *Coordinator) AuthorizationAttemptRetention() time.Duration {
	return c.installedAuthorizations.retention
}

// CreateAuthorizationAttempt validates a configured server and starts one
// component-owned interactive OAuth flow. A newer operation for the same server
// cancels this attempt; operations for other servers remain independent.
func (c *Coordinator) CreateAuthorizationAttempt(ctx context.Context, server mcpserver.ID) (AuthorizationAttempt, error) {
	write, err := c.beginMutation(ctx)
	if err != nil {
		return AuthorizationAttempt{}, err
	}
	defer write.close()
	target, err := c.connectionTarget(write.requestCtx, server)
	if err != nil {
		return AuthorizationAttempt{}, err
	}
	if target.Transport() != mcpserver.TransportStreamableHTTP {
		return AuthorizationAttempt{}, ErrAuthorizationUnsupported
	}
	var attempt AuthorizationAttempt
	var settle func(context.Context, AuthorizationAttemptStatus) error
	if server.Origin().Kind() == mcpserver.OriginUser {
		attempt, settle, err = c.external.StartAuthorization(write.requestCtx, target, newAuthorizationAttemptID())
		if err != nil {
			return AuthorizationAttempt{}, err
		}
	} else {
		attempt = c.installedAuthorizations.create(server)
		settle = func(_ context.Context, status AuthorizationAttemptStatus) error {
			c.installedAuthorizations.settle(attempt.ID, status)
			return nil
		}
	}
	if server.Origin().Kind() == mcpserver.OriginUser {
		// Starting a new attempt supersedes this server's former pending work in
		// the connector. The new operation never inherits its completion.
		c.authorizationDeliveries.Range(func(key, value any) bool {
			if value.(*authorizationDelivery).server == server {
				c.authorizationDeliveries.CompareAndDelete(key, value)
			}
			return true
		})
		original := settle
		settle = func(ctx context.Context, status AuthorizationAttemptStatus) error {
			delivery := &authorizationDelivery{server: server, status: status, send: original}
			c.authorizationDeliveries.Store(attempt.ID, delivery)
			return c.deliverAuthorization(ctx, attempt.ID, delivery)
		}
	}
	start := make(chan struct{})
	defer close(start)
	var event *statusEvent
	err = c.dispatchConnection(write.ownerCtx, server, c.connections.BeginAuthorize, start, func() { c.statusQueue.publish(event) }, func(outcome connectionOutcome) {
		if err := settle(context.WithoutCancel(write.ownerCtx), authorizationAttemptStatus(outcome)); err != nil {
			slog.WarnContext(write.ownerCtx, "mcp: authorization completion awaits confirmation", "attempt", attempt.ID.String(), "error", err)
		}
	})
	if err != nil {
		return AuthorizationAttempt{}, errors.Join(err, settle(context.WithoutCancel(write.ownerCtx), AuthorizationAttemptFailed))
	}
	event = c.statusQueue.prepare(server)
	write.unlock()
	return attempt, nil
}

// AuthorizationAttempt returns one live or retained terminal attempt.
func (c *Coordinator) AuthorizationAttempt(ctx context.Context, id string) (AuthorizationAttempt, error) {
	parsed, err := ParseAuthorizationAttemptID(id)
	if err != nil {
		return AuthorizationAttempt{}, ErrAuthorizationAttemptNotFound
	}
	attempt, ok := c.installedAuthorizations.get(parsed)
	if !ok {
		observed, err := c.external.ReadAuthorization(ctx, parsed)
		if err != nil {
			return AuthorizationAttempt{}, err
		}
		value, failed := c.authorizationDeliveries.Load(parsed)
		if !failed {
			return observed, nil
		}
		if observed.Status != AuthorizationAttemptPending {
			c.authorizationDeliveries.CompareAndDelete(parsed, value)
			return observed, nil
		}
		if err := c.deliverAuthorization(ctx, parsed, value.(*authorizationDelivery)); err != nil {
			return AuthorizationAttempt{}, err
		}
		return c.external.ReadAuthorization(ctx, parsed)
	}
	return attempt, nil
}

type authorizationDelivery struct {
	server mcpserver.ID
	status AuthorizationAttemptStatus
	send   func(context.Context, AuthorizationAttemptStatus) error
}

func (c *Coordinator) deliverAuthorization(ctx context.Context, id AuthorizationAttemptID, d *authorizationDelivery) error {
	if err := d.send(ctx, d.status); err != nil {
		return err
	}
	c.authorizationDeliveries.CompareAndDelete(id, d)
	return nil
}

func authorizationAttemptStatus(outcome connectionOutcome) AuthorizationAttemptStatus {
	switch outcome {
	case connectionSucceeded:
		return AuthorizationAttemptSucceeded
	case connectionFailed:
		return AuthorizationAttemptFailed
	case connectionCanceled:
		return AuthorizationAttemptCanceled
	default:
		panic("mcp: unknown MCP connection outcome")
	}
}

type authorizationAttemptStore struct {
	mu        sync.Mutex
	now       func() time.Time
	newID     func() AuthorizationAttemptID
	retention time.Duration
	attempts  map[AuthorizationAttemptID]AuthorizationAttempt
}

func newAuthorizationAttemptStore() *authorizationAttemptStore {
	return newAuthorizationAttemptStoreWith(
		time.Now,
		newAuthorizationAttemptID,
		authorizationAttemptRetention,
	)
}

func newAuthorizationAttemptStoreWith(
	now func() time.Time,
	newID func() AuthorizationAttemptID,
	retention time.Duration,
) *authorizationAttemptStore {
	if now == nil || newID == nil || retention <= 0 {
		panic("mcp: invalid MCP authorization attempt store configuration")
	}
	return &authorizationAttemptStore{
		now: now, newID: newID, retention: retention,
		attempts: make(map[AuthorizationAttemptID]AuthorizationAttempt),
	}
}

func (a *authorizationAttemptStore) create(server mcpserver.ID) AuthorizationAttempt {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.purgeExpiredLocked()
	id := a.newID()
	if err := id.Validate(); err != nil {
		panic("mcp: authorization attempt identity source returned an invalid identity")
	}
	attempt := AuthorizationAttempt{
		ID: id, Server: server,
		Status: AuthorizationAttemptPending, CreatedAt: a.now().UTC(),
	}
	if _, exists := a.attempts[attempt.ID]; exists {
		panic("mcp: duplicate MCP authorization attempt id")
	}
	a.attempts[attempt.ID] = attempt
	return attempt
}

func (a *authorizationAttemptStore) get(id AuthorizationAttemptID) (AuthorizationAttempt, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.purgeExpiredLocked()
	attempt, ok := a.attempts[id]
	return cloneAuthorizationAttempt(attempt), ok
}

func (a *authorizationAttemptStore) settle(id AuthorizationAttemptID, status AuthorizationAttemptStatus) {
	if !status.Valid() || status == AuthorizationAttemptPending {
		panic("mcp: invalid terminal MCP authorization attempt status")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	attempt, ok := a.attempts[id]
	if !ok || attempt.Status != AuthorizationAttemptPending {
		return
	}
	finishedAt := a.now().UTC()
	attempt.Status = status
	attempt.FinishedAt = &finishedAt
	a.attempts[id] = attempt
}

func (a *authorizationAttemptStore) purgeExpiredLocked() {
	cutoff := a.now().UTC().Add(-a.retention)
	for id, attempt := range a.attempts {
		if attempt.FinishedAt != nil && !attempt.FinishedAt.After(cutoff) {
			delete(a.attempts, id)
		}
	}
}

func cloneAuthorizationAttempt(attempt AuthorizationAttempt) AuthorizationAttempt {
	if attempt.FinishedAt != nil {
		attempt.FinishedAt = new(*attempt.FinishedAt)
	}
	return attempt
}
