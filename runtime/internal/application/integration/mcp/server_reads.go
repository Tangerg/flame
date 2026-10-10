package mcp

import (
	"context"
	"fmt"
	"slices"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
)

// Servers returns every durable MCP server enriched with the current live
// status snapshot. The registry determines membership and enablement; the
// connection supervisor determines live lifecycle independently of package realization.
func (c *Coordinator) Servers(ctx context.Context) ([]Server, error) {
	sources, err := c.registry.Catalog(ctx)
	if err != nil {
		return nil, err
	}
	slices.SortFunc(sources, func(first, second Source) int {
		return first.Server.ID().Compare(second.Server.ID())
	})
	statuses := c.statusesByName()
	out := make([]Server, 0, len(sources))
	for _, source := range sources {
		server := source.Server
		status, ok := statuses[server.ID()]
		var live *ServerStatus
		if ok {
			live = &status
		}
		view, err := serverView(server, live)
		if err != nil {
			return nil, err
		}
		out = append(out, view)
	}
	return out, nil
}

func (c *Coordinator) statusesByName() map[mcpserver.ID]ServerStatus {
	statuses := make(map[mcpserver.ID]ServerStatus)
	for _, status := range c.connections.Statuses() {
		view := statusView(status)
		statuses[view.Server] = view
	}
	return statuses
}

// ServerStatus resolves one safe live status notification read model.
func (c *Coordinator) ServerStatus(_ context.Context, name mcpserver.ID) (ServerStatus, error) {
	if err := name.Validate(); err != nil {
		return ServerStatus{}, fmt.Errorf("mcp: server status: %w", err)
	}
	if status, ok := c.statusesByName()[name]; ok {
		return status, nil
	}
	return ServerStatus{Server: name}, nil
}
