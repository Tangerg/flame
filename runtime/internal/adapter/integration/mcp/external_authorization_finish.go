package mcp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	wire "github.com/Tangerg/flame/packages/go-sdk/mcpconnector"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	mcpapp "github.com/Tangerg/flame/runtime/internal/application/integration/mcp"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/telemetry"
)

const authorizationDeliveryRetryInterval = time.Second

func (c *ExternalConnector) recordAuthorizationFinish(ctx context.Context, finish mcpapp.AuthorizationFinish) error {
	if err := c.vault.RecordAuthorizationFinish(ctx, finish); err != nil {
		return err
	}
	select {
	case c.authorizationWork <- struct{}{}:
	default:
	}
	return nil
}

func (c *ExternalConnector) RunAuthorizationDelivery(ctx context.Context) {
	for ctx.Err() == nil {
		var retry <-chan time.Time
		if err := c.deliverAuthorizationFinishes(ctx); err != nil && ctx.Err() == nil {
			slog.WarnContext(ctx, "mcp: authorization outcomes await their connector receipts", telemetry.Error(err))
			retry = time.After(authorizationDeliveryRetryInterval)
		}
		select {
		case <-ctx.Done():
		case <-c.authorizationWork:
		case <-retry:
		}
	}
}

func (c *ExternalConnector) deliverAuthorizationFinishes(ctx context.Context) error {
	c.authorizationMu.Lock()
	defer c.authorizationMu.Unlock()
	return c.deliverAuthorizationFinishesLocked(ctx, nil)
}

func (c *ExternalConnector) deliverAuthorizationFinishesLocked(ctx context.Context, scope *connectorUse) error {
	finishes, err := c.vault.PendingAuthorizationFinishes(ctx)
	if err != nil {
		return err
	}
	var result error
	for _, pending := range finishes {
		u := scope
		if scope != nil && pending.Route.Source != scope.source.ContributionSource() {
			continue
		}
		if u == nil {
			u, err = c.acquireCaptured(ctx, pending.Route.Source)
			if err != nil {
				result = errors.Join(result, fmt.Errorf("mcp: borrow the owner of %s: %w", pending.ID, err))
				continue
			}
		}
		err := c.settleAuthorizationFinish(ctx, u, pending)
		if scope == nil {
			err = errors.Join(err, u.session.release(context.WithoutCancel(ctx)))
		}
		if err != nil {
			result = errors.Join(result, fmt.Errorf("mcp: settle %s: %w", pending.ID, err))
		}
	}
	return result
}

func (c *ExternalConnector) settleAuthorizationFinish(ctx context.Context, u *connectorUse, pending mcpapp.PendingAuthorizationFinish) error {
	ctx, method, err := bindConnectorCommand(ctx, u, pending.Route.Source, pending.Invocation, c.methods.Finish)
	if err != nil {
		return err
	}
	id := pluginmcp.CommandID(pending.Invocation.ID)
	input := wire.Completion{ID: pending.ID.String(), Epoch: pending.Route.Epoch, Status: pending.Outcome}
	decision, found, err := method.ReadReceipt(ctx, id, input)
	if err != nil {
		return err
	}
	if !found {
		decision, err = method.CallCommand(ctx, id, input)
		if err != nil {
			return err
		}
	}
	switch decision {
	case wire.Applied, wire.Conflict:
		return c.vault.CompleteAuthorizationFinish(ctx, pending.ID)
	default:
		return mcpapp.ErrInvalidServerConfiguration
	}
}

func (c *ExternalConnector) restoreAuthorizationEpochs(ctx context.Context) error {
	connectors, err := c.vault.AuthorizationConnectors(ctx)
	if err != nil {
		return err
	}
	var result error
	for _, source := range connectors {
		u, err := c.acquireCaptured(ctx, source)
		if errors.Is(err, plugin.ErrUnavailable) {
			continue
		}
		if err != nil {
			result = errors.Join(result, err)
			continue
		}
		result = errors.Join(result, c.recoverAuthorizationEpoch(ctx, u), u.session.release(context.WithoutCancel(ctx)))
	}
	return result
}
