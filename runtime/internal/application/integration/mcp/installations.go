package mcp

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
)

// ReconcileInstallation realizes committed installation projections in the
// existing supervisor. It cannot save or edit an installation descriptor.
func (c *Coordinator) ReconcileInstallation(ctx context.Context, names []mcpserver.ServerName) error {
	for _, name := range names {
		if err := name.Validate(); err != nil {
			return fmt.Errorf("%w: installation source: %w", ErrInvalidServerConfiguration, err)
		}
		if name.Installation() == "" {
			return fmt.Errorf("%w: installation source is required", ErrInvalidServerConfiguration)
		}
	}
	write, err := c.beginMutation(ctx)
	if err != nil {
		return err
	}
	defer write.close()
	var result error
	var events []*statusEvent
	start := make(chan struct{})
	defer close(start)
	for _, name := range names {
		// Read the current owner under the supervisor's sequencing lock. A
		// delayed reconcile may never dispatch a superseded command snapshot.
		server, found, err := c.registry.Definition(write.ownerCtx, name)
		if err != nil {
			result = errors.Join(result, err)
			c.cancelDial(name)
			result = errors.Join(result, c.connectionLifecycle.Detach(name))
			events = append(events, c.prepareStatus(ServerStatus{Name: name}, nil))
			continue
		}
		c.cancelDial(name)
		if !found || !server.Enabled {
			result = errors.Join(result, c.connectionLifecycle.Detach(name))
		}
		if !found {
			c.exposure.removeServer(name)
			events = append(events, c.prepareStatus(ServerStatus{Name: name}, nil))
			continue
		}
		c.exposure.setServer(server)
		status := ServerStatus{Name: server.Name}
		var operation *activeDial
		if server.Enabled {
			operation, err = c.redialServer(write.ownerCtx, server.Name, start)
			if err != nil {
				result = errors.Join(result, err)
			} else {
				status.State = mcpserver.ConnectionConnecting
				status.Known = true
			}
		}
		events = append(events, c.prepareStatus(status, operation))
	}
	write.unlock()
	for _, event := range events {
		c.statusQueue.publish(event)
	}
	return result
}
