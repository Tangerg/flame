package mcp

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
)

// Servers returns every durable MCP server enriched with the current live
// status snapshot. The registry determines membership; the live pool is only a
// projection and therefore cannot make a configured or disabled server vanish.
func (c *Coordinator) Servers(ctx context.Context) ([]Server, error) {
	if c.registry == nil {
		return nil, errors.New("mcp: MCP registry is unavailable")
	}
	servers, err := c.registry.List(ctx)
	if err != nil {
		return nil, err
	}
	servers, err = validateRegistryCatalog(servers)
	if err != nil {
		return nil, err
	}
	slices.SortFunc(servers, func(first, second mcpserver.Server) int {
		return cmp.Compare(first.Name.String(), second.Name.String())
	})
	statuses, err := c.statusesByName()
	if err != nil {
		return nil, err
	}
	out := make([]Server, 0, len(servers))
	for _, server := range servers {
		status, ok := statuses[server.Name]
		if ok {
			out = append(out, serverView(server, &status))
		} else {
			out = append(out, serverView(server, nil))
		}
	}
	return out, nil
}

// Server returns one unified server resource.
func (c *Coordinator) Server(ctx context.Context, name mcpserver.ServerName) (Server, error) {
	if c.registry == nil {
		return Server{}, errors.New("mcp: MCP registry is unavailable")
	}
	server, found, err := c.registry.Get(ctx, name)
	if err != nil {
		return Server{}, err
	}
	if !found {
		return Server{}, ErrUnknownServer
	}
	server = server.Clone()
	if err := validateRegistryServer("get", name, server); err != nil {
		return Server{}, err
	}
	statuses, err := c.statusesByName()
	if err != nil {
		return Server{}, err
	}
	status, ok := statuses[name]
	if ok {
		return serverView(server, &status), nil
	}
	return serverView(server, nil), nil
}

func validateRegistryCatalog(servers []mcpserver.Server) ([]mcpserver.Server, error) {
	owned := make([]mcpserver.Server, len(servers))
	for index, server := range servers {
		owned[index] = server.Clone()
	}
	servers = owned
	seen := make(map[mcpserver.ServerName]struct{}, len(servers))
	for index, server := range servers {
		if err := server.Validate(); err != nil {
			return nil, fmt.Errorf("mcp: registry row %d is invalid: %w", index+1, err)
		}
		if _, duplicate := seen[server.Name]; duplicate {
			return nil, fmt.Errorf("mcp: registry repeats server %q", server.Name)
		}
		seen[server.Name] = struct{}{}
	}
	return servers, nil
}

func validateRegistryServer(operation string, expected mcpserver.ServerName, server mcpserver.Server) error {
	if err := server.Validate(); err != nil {
		return fmt.Errorf("mcp: registry %s for %q returned an invalid server: %w", operation, expected, err)
	}
	if server.Name != expected {
		return fmt.Errorf("mcp: registry %s for %q returned %q", operation, expected, server.Name)
	}
	return nil
}

func (c *Coordinator) statusesByName() (map[mcpserver.ServerName]ServerStatus, error) {
	statuses, err := c.liveStatusesByName()
	if err != nil {
		return nil, err
	}
	c.statusMu.Lock()
	defer c.statusMu.Unlock()
	for name, status := range c.statusOverrides {
		if err := status.Validate(); err != nil {
			return nil, fmt.Errorf("mcp: status override for %q is invalid: %w", name, err)
		}
		if status.Name != name {
			return nil, fmt.Errorf("mcp: status override for %q belongs to %q", name, status.Name)
		}
		if !status.Known {
			if _, staleLiveEntry := statuses[name]; !staleLiveEntry {
				// The live port has caught up with disable/delete. Absence already
				// projects as unknown, so the tombstone has finished its handoff.
				delete(c.statusOverrides, name)
				continue
			}
		}
		statuses[name] = cloneServerStatus(status)
	}
	return statuses, nil
}

// liveStatusesByName reads the status-port projection without the application's
// transition overlay. Connection settlement must use this source: reading the
// public model there would merely observe the synthetic connecting state that
// the same operation published before dialing.
func (c *Coordinator) liveStatusesByName() (map[mcpserver.ServerName]ServerStatus, error) {
	statuses := make(map[mcpserver.ServerName]ServerStatus)
	if c.statusReader == nil {
		return statuses, nil
	}
	for index, status := range c.statusReader.Statuses() {
		view, err := statusView(status)
		if err != nil {
			return nil, fmt.Errorf("mcp: live status row %d is invalid: %w", index+1, err)
		}
		if _, duplicate := statuses[view.Name]; duplicate {
			return nil, fmt.Errorf("mcp: live status catalog repeats server %q", view.Name)
		}
		statuses[view.Name] = view
	}
	return statuses, nil
}

func (c *Coordinator) liveStatus(name mcpserver.ServerName) (ServerStatus, error) {
	if err := name.Validate(); err != nil {
		return ServerStatus{}, fmt.Errorf("mcp: live status server: %w", err)
	}
	statuses, err := c.liveStatusesByName()
	if err != nil {
		return ServerStatus{}, err
	}
	if status, ok := statuses[name]; ok {
		return status, nil
	}
	return ServerStatus{Name: name}, nil
}

// ServerStatus resolves one safe live status notification read model.
func (c *Coordinator) ServerStatus(_ context.Context, name mcpserver.ServerName) (ServerStatus, error) {
	if err := name.Validate(); err != nil {
		return ServerStatus{}, fmt.Errorf("mcp: server status: %w", err)
	}
	statuses, err := c.statusesByName()
	if err != nil {
		return ServerStatus{}, err
	}
	if status, ok := statuses[name]; ok {
		return status, nil
	}
	return ServerStatus{Name: name}, nil
}

// acceptStatus makes a transition readable before publishing its invalidation.
// The live status port remains the cold-start source; this overlay owns only
// transitions admitted by this Coordinator, including the synthetic connecting
// state that precedes the connection call.
func (c *Coordinator) acceptStatus(status ServerStatus) {
	c.statusMu.Lock()
	if status.Known && status.State != mcpserver.ConnectionConnecting {
		// Terminal connection states already come from the live status port. Drop
		// the temporary connecting overlay instead of copying that terminal fact
		// into a second long-lived source that could hide later passive changes.
		delete(c.statusOverrides, status.Name)
	} else {
		// Unknown is a tombstone that masks a stale live snapshot after
		// disable/delete; connecting is the application-owned pre-dial transition.
		c.statusOverrides[status.Name] = cloneServerStatus(status)
	}
	c.statusMu.Unlock()
	c.invalidations.Notify(invalidation.ForMCP(status.Name.String()))
}

func cloneServerStatus(status ServerStatus) ServerStatus {
	if status.ToolCount != nil {
		count := *status.ToolCount
		status.ToolCount = &count
	}
	return status
}
