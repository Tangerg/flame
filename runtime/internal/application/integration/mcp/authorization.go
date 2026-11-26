package mcp

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/mcpconnector"

	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/telemetry"
)

const AuthorizationAttemptRetention = mcpconnector.AttemptRetentionSeconds * time.Second

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
	return AuthorizationAttemptRetention
}

func (c *Coordinator) StartAuthorization(ctx context.Context, server mcpserver.ID, receipt operation.ReplayClaim) (AuthorizationAttemptID, error) {
	write, err := c.beginMutation(ctx)
	if err != nil {
		return AuthorizationAttemptID{}, err
	}
	defer write.close()
	target, err := c.authorizationTarget(write.requestCtx, server)
	if err != nil {
		return AuthorizationAttemptID{}, err
	}
	settlement, err := c.external.StartAuthorization(write.requestCtx, target, newAuthorizationAttemptID(), receipt)
	if err != nil {
		return AuthorizationAttemptID{}, err
	}
	return settlement.AttemptID, c.completeAuthorizationStart(write, settlement)
}

func (c *Coordinator) RecoverAuthorization(ctx context.Context, receipt operation.ReplayClaim) error {
	write, err := c.beginMutation(ctx)
	if err != nil {
		return err
	}
	defer write.close()
	settlement, err := c.external.RecoverAuthorizationStart(write.requestCtx, receipt)
	if err != nil {
		return err
	}
	if settlement == nil {
		return operation.ErrInProgress
	}
	return c.completeAuthorizationStart(write, settlement)
}

func (c *Coordinator) completeAuthorizationStart(write *mutationScope, settlement *AuthorizationStartSettlement) error {
	// The receipt must outlive the interactive transport, which cannot be resumed after a crash.
	if err := settlement.Complete(write.ownerCtx); err != nil {
		return err
	}
	if settlement.Finish != nil {
		c.authorizationDeliveries.forgetServer(settlement.Server)
		c.launchAuthorization(write, settlement)
	}
	return settlement.Refusal
}

func (c *Coordinator) authorizationTarget(ctx context.Context, server mcpserver.ID) (mcpserver.Server, error) {
	target, err := c.connectionTarget(ctx, server)
	if err != nil {
		return mcpserver.Server{}, err
	}
	if _, eligible := target.OAuthTarget(); server.Origin().Kind() != mcpserver.OriginUser || !eligible {
		return mcpserver.Server{}, ErrAuthorizationUnsupported
	}
	return target, nil
}

func (c *Coordinator) launchAuthorization(write *mutationScope, settlement *AuthorizationStartSettlement) {
	server, attempt := settlement.Server, settlement.AttemptID
	start := make(chan struct{})
	defer close(start)
	var event *statusEvent
	finish := func(status AuthorizationAttemptStatus) {
		delivery := &authorizationDelivery{server: server, status: status, send: settlement.Finish}
		c.authorizationDeliveries.record(attempt, delivery)
		if err := c.deliverAuthorization(context.WithoutCancel(write.ownerCtx), attempt, delivery); err != nil {
			slog.WarnContext(write.ownerCtx, "mcp: authorization completion awaits confirmation", slog.Any("attempt", attempt.String()), telemetry.Error(err))
		}
	}
	err := c.dispatchConnection(write.ownerCtx, server, c.connections.BeginAuthorize, start, func() { c.statusQueue.publish(event) }, func(outcome connectionOutcome) { finish(authorizationAttemptStatus(outcome)) })
	if err != nil {
		slog.ErrorContext(write.ownerCtx, "mcp: start accepted authorization", telemetry.MCPServer(server.String()), telemetry.Error(err))
		finish(AuthorizationAttemptFailed)
		return
	}
	event = c.statusQueue.prepare(server)
	write.unlock()
}

func (c *Coordinator) AuthorizationAttempt(ctx context.Context, id string) (AuthorizationAttempt, error) {
	parsed, err := ParseAuthorizationAttemptID(id)
	if err != nil {
		return AuthorizationAttempt{}, ErrAuthorizationAttemptNotFound
	}
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
