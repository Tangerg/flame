package mcp

import (
	"context"
	"log/slog"
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

func (c *Coordinator) StartAuthorization(ctx context.Context, server mcpserver.ID, receipt operation.ReplayClaim) error {
	write, err := c.beginMutation(ctx)
	if err != nil {
		return err
	}
	defer write.close()
	target, err := c.authorizationTarget(write.requestCtx, server)
	if err != nil {
		return err
	}
	settlement, err := c.external.StartAuthorization(write.requestCtx, target, newAuthorizationAttemptID(), receipt)
	if err != nil {
		return err
	}
	return c.completeAuthorizationStart(write, settlement)
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
	if settlement.Transport != nil {
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
	server, attempt, transport := settlement.Server, settlement.AttemptID, settlement.Transport
	finish := func(status AuthorizationAttemptStatus) {
		if err := transport.Report(context.WithoutCancel(write.ownerCtx), status); err != nil {
			slog.ErrorContext(write.ownerCtx, "mcp: record authorization outcome", slog.String("attempt", attempt.String()), telemetry.Error(err))
		}
	}
	begin := func(ctx context.Context) (Attempt, error) {
		return c.connections.BeginAuthorize(ctx, server, transport.Recipient)
	}
	start := make(chan struct{})
	defer close(start)
	var event *statusEvent
	err := c.dispatchConnection(write.ownerCtx, server, begin, start, func() { c.statusQueue.publish(event) }, func(outcome connectionOutcome) { finish(authorizationAttemptStatus(outcome)) })
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
	return c.external.ReadAuthorization(ctx, parsed)
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
