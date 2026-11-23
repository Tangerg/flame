package mcp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/telemetry"
)

func (c *Coordinator) WithdrawInstallation(ctx context.Context, servers []mcpserver.ID) error {
	if ctx == nil {
		return errors.New("mcp: withdrawal context is required")
	}
	if err := validateInstallationSources(servers); err != nil {
		return err
	}
	affected, result := c.external.Affected(ctx, servers)
	if result != nil || affected {
		result = errors.Join(result, c.WithdrawExternal())
	}
	for _, id := range servers {
		result = errors.Join(result, c.connections.Detach(id))
	}
	return result
}

func (c *Coordinator) ReconcileInstallation(ctx context.Context, servers []mcpserver.ID) error {
	if ctx == nil {
		return errors.New("mcp: realization context is required")
	}
	if err := validateInstallationSources(servers); err != nil {
		return err
	}
	affected, selectionErr := c.external.Affected(ctx, servers)
	result := c.reconcileSources(ctx, servers)
	if selectionErr != nil {
		return errors.Join(result, selectionErr)
	}
	if affected {
		result = errors.Join(result, c.ReconcileExternal(ctx))
	}
	return result
}

func (c *Coordinator) RestoreExternal(ctx context.Context, ready <-chan struct{}) {
	c.tasks.StartLinked(ctx, func(ctx context.Context) {
		select {
		case <-ctx.Done():
			return
		case <-ready:
		}
		if err := c.ReconcileExternal(ctx); err != nil && ctx.Err() == nil {
			slog.WarnContext(ctx, "external MCP directory could not be restored", telemetry.Error(err))
		}
	})
}

func (c *Coordinator) WithdrawExternal() error {
	c.external.Invalidate()
	var err error
	for _, s := range c.connections.Statuses() {
		if s.Server.Origin().Kind() == mcpserver.OriginUser {
			err = errors.Join(err, c.connections.Detach(s.Server))
		}
	}
	return err
}

func (c *Coordinator) ReconcileExternal(ctx context.Context) error {
	definitions, err := c.external.List(ctx)
	if err != nil {
		return err
	}
	ids := make([]mcpserver.ID, 0, len(definitions))
	for _, s := range definitions {
		ids = append(ids, s.ID())
	}
	return c.reconcileSources(ctx, ids)
}

func (c *Coordinator) reconcileSources(ctx context.Context, servers []mcpserver.ID) error {
	write, err := c.beginMutation(ctx)
	if err != nil {
		return errors.Join(err, c.refuseUnrealized(ctx, servers))
	}
	defer write.close()
	var result error
	var events []*statusEvent
	start := make(chan struct{})
	defer close(start)
	for _, id := range servers {
		// Read the current owner under the supervisor's sequencing lock. A
		// delayed reconcile may never dispatch a superseded command snapshot.
		server, found, err := c.registry.Definition(write.ownerCtx, id)
		if err != nil {
			result = errors.Join(result, err, c.connections.Refuse(write.ownerCtx, id, mcpserver.FailureConfiguration))
			events = append(events, c.statusQueue.prepare(id))
			continue
		}
		if !found || !server.Enabled {
			result = errors.Join(result, c.connections.Detach(id))
		}
		if !found {
			c.exposure.removeServer(id)
			events = append(events, c.statusQueue.prepare(id))
			continue
		}
		c.exposure.setServer(server)
		if server.Enabled {
			result = errors.Join(result, c.dispatchConnection(write.ownerCtx, server.ID(), c.connections.BeginConnect, start, nil, nil))
		}
		events = append(events, c.statusQueue.prepare(id))
	}
	write.unlock()
	for _, event := range events {
		c.statusQueue.publish(event)
	}
	return result
}

func (c *Coordinator) refuseUnrealized(ctx context.Context, servers []mcpserver.ID) error {
	ctx = context.WithoutCancel(ctx)
	var result error
	for _, id := range servers {
		server, found, err := c.registry.Definition(ctx, id)
		if err != nil {
			result = errors.Join(result, err)
			continue
		}
		if found && server.Enabled {
			result = errors.Join(result, c.connections.Refuse(ctx, id, mcpserver.FailureConfiguration))
		}
		c.invalidations.Notify(invalidation.ForMCP(id))
	}
	return result
}

func validateInstallationSources(servers []mcpserver.ID) error {
	for _, id := range servers {
		if err := id.Validate(); err != nil {
			return fmt.Errorf("%w: installation source: %w", ErrInvalidServerConfiguration, err)
		}
		if id.Origin().Kind() != mcpserver.OriginInstallation {
			return fmt.Errorf("%w: installation source is required", ErrInvalidServerConfiguration)
		}
	}
	return nil
}
