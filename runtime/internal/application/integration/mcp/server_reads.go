package mcp

import (
	"context"
	"slices"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
)

func (c *Coordinator) Servers(ctx context.Context) ([]Server, error) {
	servers, err := c.registry.Definitions(ctx)
	if err != nil {
		return nil, err
	}
	slices.SortFunc(servers, func(first, second mcpserver.Server) int {
		return first.ID().Compare(second.ID())
	})
	statuses := c.statusesByName()
	out := make([]Server, 0, len(servers))
	for _, server := range servers {
		view, err := serverView(server, statuses[server.ID()])
		if err != nil {
			return nil, err
		}
		out = append(out, view)
	}
	return out, nil
}

func (c *Coordinator) statusesByName() map[mcpserver.ID]*mcpserver.ConnectionStatus {
	statuses := make(map[mcpserver.ID]*mcpserver.ConnectionStatus)
	for _, status := range c.connections.Statuses() {
		statuses[status.Server] = &status
	}
	return statuses
}
