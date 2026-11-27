package mcp

import (
	"context"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
)

func (c *Coordinator) ToolExposure(ctx context.Context, server mcpserver.ID) ([]tool.Ref, error) {
	if _, found, err := c.registry.Definition(ctx, server); err != nil {
		return nil, err
	} else if !found {
		return nil, ErrUnknownServer
	}
	refs, err := c.exposureStore.ListExposure(ctx)
	if err != nil {
		return nil, err
	}
	var selected []tool.Ref
	for _, ref := range refs {
		if source, _, _ := ref.MCP(); source == server {
			selected = append(selected, ref)
		}
	}
	return selected, nil
}

func (c *Coordinator) SetToolExposure(ctx context.Context, ref tool.Ref, disabled bool, complete operation.Reply[struct{}]) error {
	server, _, ok := ref.MCP()
	if !ok {
		return fmt.Errorf("%w: exposure requires an MCP reference", ErrInvalidServerConfiguration)
	}
	write, err := c.beginMutation(ctx)
	if err != nil {
		return err
	}
	defer write.close()
	if _, found, err := c.registry.Definition(write.requestCtx, server); err != nil {
		return err
	} else if !found {
		return ErrUnknownServer
	}
	if err := complete(write.requestCtx, func(tx context.Context) (struct{}, error) {
		return struct{}{}, c.exposureStore.SetToolExposure(tx, ref, disabled)
	}); err != nil {
		return err
	}
	write.unlock()
	c.invalidations.Notify(invalidation.Notice{Resource: invalidation.MCP})
	return nil
}
