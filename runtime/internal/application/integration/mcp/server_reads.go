package mcp

import (
	"cmp"
	"context"
	"fmt"
	"slices"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
)

// Servers returns every durable MCP server enriched with the current live
// status snapshot. The registry determines membership; the live pool is only a
// projection and therefore cannot make a configured or disabled server vanish.
func (c *Coordinator) Servers(ctx context.Context) ([]Server, error) {
	sources, err := c.registry.Catalog(ctx)
	if err != nil {
		return nil, err
	}
	slices.SortFunc(sources, func(first, second Source) int {
		return cmp.Compare(first.Server.Name.String(), second.Server.Name.String())
	})
	statuses := c.statusesByName()
	out := make([]Server, 0, len(sources))
	for _, source := range sources {
		server := source.Server
		status, ok := statuses[server.Name]
		var live *ServerStatus
		if ok {
			live = &status
		}
		view, err := serverView(server, live)
		if err != nil {
			return nil, err
		}
		switch source.Availability {
		case SourceAvailable:
		case SourceUnavailableRelease, SourceUnavailableBackend:
			if server.Enabled {
				view.State = ServerState{Type: ServerFailed}
			}
		default:
			return nil, fmt.Errorf("mcp: unknown source availability %q", source.Availability)
		}
		out = append(out, view)
	}
	return out, nil
}

func (c *Coordinator) statusesByName() map[mcpserver.ServerName]ServerStatus {
	statuses := c.liveStatusesByName()
	c.connectionMu.Lock()
	defer c.connectionMu.Unlock()
	for name := range c.statusTombstones {
		if _, staleLiveEntry := statuses[name]; !staleLiveEntry {
			// Absence completes the disable/delete handoff to the live port.
			delete(c.statusTombstones, name)
			continue
		}
		statuses[name] = ServerStatus{Name: name}
	}
	for name, dial := range c.dials {
		if dial.connecting {
			statuses[name] = ServerStatus{Name: name, Known: true, State: mcpserver.ConnectionConnecting}
		}
	}
	return statuses
}

// liveStatusesByName reads the status-port projection without the
// application's pending dials. Settlement reads this source instead of the
// connecting phase of the same operation.
func (c *Coordinator) liveStatusesByName() map[mcpserver.ServerName]ServerStatus {
	statuses := make(map[mcpserver.ServerName]ServerStatus)
	for _, status := range c.statusReader.Statuses() {
		view := statusView(status)
		statuses[view.Name] = view
	}
	return statuses
}

func (c *Coordinator) liveStatus(name mcpserver.ServerName) (ServerStatus, error) {
	if err := name.Validate(); err != nil {
		return ServerStatus{}, fmt.Errorf("mcp: live status server: %w", err)
	}
	if status, ok := c.liveStatusesByName()[name]; ok {
		return status, nil
	}
	return ServerStatus{Name: name}, nil
}

// ServerStatus resolves one safe live status notification read model.
func (c *Coordinator) ServerStatus(_ context.Context, name mcpserver.ServerName) (ServerStatus, error) {
	if err := name.Validate(); err != nil {
		return ServerStatus{}, fmt.Errorf("mcp: server status: %w", err)
	}
	if status, ok := c.statusesByName()[name]; ok {
		return status, nil
	}
	return ServerStatus{Name: name}, nil
}

// prepareStatus advances the identified operation while mutationMu is held.
// Notifications carry only source identity: a delayed consumer cannot advance
// connection facts after their operation has retired.
func (c *Coordinator) prepareStatus(status ServerStatus, operation *activeDial) *statusEvent {
	c.connectionMu.Lock()
	if operation != nil {
		if c.dials[status.Name] != operation {
			c.connectionMu.Unlock()
			return nil
		}
		operation.connecting = status.Known && status.State == mcpserver.ConnectionConnecting
	}
	if status.Known {
		delete(c.statusTombstones, status.Name)
	} else {
		c.statusTombstones[status.Name] = struct{}{}
	}
	c.connectionMu.Unlock()
	return c.statusQueue.prepare(status.Name)
}
