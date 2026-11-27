package mcp

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/telemetry"
)

func (c *Coordinator) CreateServer(ctx context.Context, input ServerInput, receipt operation.ReplayClaim) error {
	id, err := userServer(input.Name)
	if err != nil {
		return err
	}
	write, err := c.beginMutation(ctx)
	if err != nil {
		return err
	}
	defer write.close()
	if _, found, getErr := c.registry.Definition(write.requestCtx, id); getErr != nil {
		return getErr
	} else if found {
		return ErrServerAlreadyExists
	}
	srv, err := serverCandidate(input, nil)
	if err != nil {
		return err
	}
	return c.saveServer(write, srv, receipt)
}

type DirectoryTarget struct {
	Server   mcpserver.ID
	Revision string
}

func (c *Coordinator) UpdateServer(ctx context.Context, target DirectoryTarget, patch ServerPatch, receipt operation.ReplayClaim) error {
	if patch.Empty() {
		return fmt.Errorf("%w: update contains no changes", ErrInvalidServerConfiguration)
	}
	write, err := c.beginMutation(ctx)
	if err != nil {
		return err
	}
	defer write.close()
	current, err := c.directoryServer(write.requestCtx, target)
	if err != nil {
		return err
	}
	updated, err := applyServerPatch(current, patch)
	if err != nil {
		return err
	}
	return c.saveServer(write, updated, receipt)
}

func (c *Coordinator) saveServer(write *mutationScope, accepted mcpserver.Server, receipt operation.ReplayClaim) error {
	settlement, err := c.directory.Save(write.requestCtx, accepted, receipt)
	if err != nil {
		return err
	}
	start := make(chan struct{})
	defer close(start)
	accepted.Source = settlement.Source
	return c.completeDirectoryChange(write, settlement, func(ctx context.Context) error {
		return c.realizeServer(ctx, accepted, start)
	})
}

func (c *Coordinator) DeleteServer(ctx context.Context, target DirectoryTarget, receipt operation.ReplayClaim) error {
	write, err := c.beginMutation(ctx)
	if err != nil {
		return err
	}
	defer write.close()
	current, err := c.directoryServer(write.requestCtx, target)
	if err != nil {
		return err
	}
	settlement, err := c.directory.Remove(write.requestCtx, current, receipt)
	if err != nil {
		return err
	}
	return c.completeDirectoryChange(write, settlement, func(context.Context) error {
		return c.connections.Detach(target.Server)
	})
}

func (c *Coordinator) directoryServer(ctx context.Context, target DirectoryTarget) (mcpserver.Server, error) {
	if target.Server.Origin().Kind() != mcpserver.OriginUser {
		return mcpserver.Server{}, ErrOwnedByInstallation
	}
	current, found, err := c.registry.Definition(ctx, target.Server)
	if err != nil {
		return mcpserver.Server{}, err
	}
	if !found {
		return mcpserver.Server{}, ErrUnknownServer
	}
	if current.DirectoryRevision != target.Revision {
		return mcpserver.Server{}, ErrDirectoryRevisionConflict
	}
	return current, nil
}

func (c *Coordinator) RecoverDirectoryChange(ctx context.Context, receipt operation.ReplayClaim) error {
	write, err := c.beginMutation(ctx)
	if err != nil {
		return err
	}
	defer write.close()
	settlement, err := c.directory.RecoverDirectoryChange(write.requestCtx, receipt)
	if err != nil {
		return err
	}
	if settlement == nil {
		return operation.ErrInProgress
	}
	return c.completeRecoveredDirectoryChange(write, settlement)
}

func (c *Coordinator) completeRecoveredDirectoryChange(write *mutationScope, settlement *DirectorySettlement) error {
	start := make(chan struct{})
	defer close(start)
	return c.completeDirectoryChange(write, settlement, func(ctx context.Context) error {
		name := settlement.ServerID()
		current, found, err := c.registry.Definition(ctx, name)
		if err != nil {
			return err
		}
		if !found {
			return c.connections.Detach(name)
		}
		return c.realizeServer(ctx, current, start)
	})
}

func (c *Coordinator) completeDirectoryChange(write *mutationScope, settlement *DirectorySettlement, project func(context.Context) error) error {
	if settlement.Refusal != nil {
		if err := settlement.Complete(write.ownerCtx); err != nil {
			return err
		}
		return settlement.Refusal
	}
	if err := project(write.ownerCtx); err != nil {
		slog.ErrorContext(write.ownerCtx, "mcp: realize accepted directory change", telemetry.MCPServer(settlement.ServerID().String()), telemetry.Error(err))
	}
	event := c.statusQueue.prepare(settlement.ServerID())
	err := settlement.Complete(write.ownerCtx)
	write.unlock()
	c.statusQueue.publish(event)
	return err
}
