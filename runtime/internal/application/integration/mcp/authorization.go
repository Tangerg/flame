package mcp

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/mcpconnector"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/telemetry"
)

const (
	authorizationAttemptRetention = mcpconnector.AttemptRetentionSeconds * time.Second
)

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

type AuthorizationAttempt struct {
	ID         AuthorizationAttemptID
	Server     mcpserver.ID
	Status     AuthorizationAttemptStatus
	CreatedAt  time.Time
	FinishedAt *time.Time
}

func (c *Coordinator) AuthorizationAttemptRetention() time.Duration {
	return c.installedAuthorizations.retention
}

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
	var settlement authorizationSettlement
	if server.Origin().Kind() == mcpserver.OriginUser {
		var send func(context.Context, AuthorizationAttemptStatus) error
		attempt, send, err = c.external.StartAuthorization(write.requestCtx, target, newAuthorizationAttemptID())
		if err != nil {
			return AuthorizationAttempt{}, err
		}
		c.authorizationDeliveries.forgetServer(server)
		settlement = externalSettlement{coordinator: c, server: server, attempt: attempt.ID, send: send}
	} else {
		attempt = c.installedAuthorizations.create(server)
		settlement = installedSettlement{store: c.installedAuthorizations, attempt: attempt.ID}
	}
	start := make(chan struct{})
	defer close(start)
	var event *statusEvent
	err = c.dispatchConnection(write.ownerCtx, server, c.connections.BeginAuthorize, start, func() { c.statusQueue.publish(event) }, func(outcome connectionOutcome) {
		if err := settlement.settle(context.WithoutCancel(write.ownerCtx), authorizationAttemptStatus(outcome)); err != nil {
			slog.WarnContext(write.ownerCtx, "mcp: authorization completion awaits confirmation", slog.Any("attempt", attempt.ID.String()), telemetry.Error(err))
		}
	})
	if err != nil {
		return AuthorizationAttempt{}, errors.Join(err, settlement.settle(context.WithoutCancel(write.ownerCtx), AuthorizationAttemptFailed))
	}
	event = c.statusQueue.prepare(server)
	write.unlock()
	return attempt, nil
}

// A user server's outcome is owned by the external connector; an installed server's by the Runtime store.
type authorizationSettlement interface {
	settle(ctx context.Context, status AuthorizationAttemptStatus) error
}

type externalSettlement struct {
	coordinator *Coordinator
	server      mcpserver.ID
	attempt     AuthorizationAttemptID
	send        func(context.Context, AuthorizationAttemptStatus) error
}

func (e externalSettlement) settle(ctx context.Context, status AuthorizationAttemptStatus) error {
	delivery := &authorizationDelivery{server: e.server, status: status, send: e.send}
	e.coordinator.authorizationDeliveries.record(e.attempt, delivery)
	return e.coordinator.deliverAuthorization(ctx, e.attempt, delivery)
}

type installedSettlement struct {
	store   *authorizationAttemptStore
	attempt AuthorizationAttemptID
}

func (i installedSettlement) settle(_ context.Context, status AuthorizationAttemptStatus) error {
	i.store.settle(i.attempt, status)
	return nil
}

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
		delivery, undelivered := c.authorizationDeliveries.pending(parsed)
		if !undelivered {
			return observed, nil
		}
		if observed.Status != AuthorizationAttemptPending {
			c.authorizationDeliveries.retire(parsed, delivery)
			return observed, nil
		}
		if err := c.deliverAuthorization(ctx, parsed, delivery); err != nil {
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
	c.authorizationDeliveries.retire(id, d)
	return nil
}

// An external outcome stays pending until the connector confirms it, so a later read can retry delivery.
type pendingDeliveries struct {
	mu        sync.Mutex
	byAttempt map[AuthorizationAttemptID]*authorizationDelivery
}

func (p *pendingDeliveries) forgetServer(server mcpserver.ID) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for id, delivery := range p.byAttempt {
		if delivery.server == server {
			delete(p.byAttempt, id)
		}
	}
}

func (p *pendingDeliveries) record(id AuthorizationAttemptID, delivery *authorizationDelivery) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.byAttempt == nil {
		p.byAttempt = make(map[AuthorizationAttemptID]*authorizationDelivery)
	}
	p.byAttempt[id] = delivery
}

func (p *pendingDeliveries) pending(id AuthorizationAttemptID) (*authorizationDelivery, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delivery, ok := p.byAttempt[id]
	return delivery, ok
}

func (p *pendingDeliveries) retire(id AuthorizationAttemptID, delivery *authorizationDelivery) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.byAttempt[id] == delivery {
		delete(p.byAttempt, id)
	}
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
