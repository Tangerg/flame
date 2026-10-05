package mcp

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
)

// WithdrawInstallation retires the live connections and tools of installation
// sources whose committed projection changed. The installation owner calls it
// inside the critical section that commits the change, after the durable
// write, so no Run assembled after the commit can freeze a superseded
// release's tools. Each source's in-flight connection operation is canceled
// before the source is detached: a configuration read before the commit is
// superseded and cannot dial afterwards. Withdrawal performs no I/O and never
// waits for a dial; ReconcileInstallation realizes the committed projection
// after the critical section.
func (c *Coordinator) WithdrawInstallation(servers []mcpserver.ID) error {
	if err := validateInstallationSources(servers); err != nil {
		return err
	}
	var result error
	for _, id := range servers {
		c.cancelDial(id)
		result = errors.Join(result, c.connectionLifecycle.Detach(id))
	}
	return result
}

// ReconcileInstallation realizes committed installation projections in the
// existing supervisor. It cannot save or edit an installation descriptor.
func (c *Coordinator) ReconcileInstallation(ctx context.Context, servers []mcpserver.ID) error {
	if err := validateInstallationSources(servers); err != nil {
		return err
	}
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
		c.cancelDial(id)
		if err != nil {
			// An unverifiable source must not read as merely disconnected
			// while nothing will dial it; like a dispatch that cannot read its
			// source, the refusal reaches status readers.
			result = errors.Join(result, err, c.connectionLifecycle.Refuse(write.ownerCtx, id, mcpserver.FailureConfiguration))
			status, statusErr := c.liveStatus(id)
			if statusErr != nil {
				result = errors.Join(result, statusErr)
				continue
			}
			events = append(events, c.prepareStatus(status, nil))
			continue
		}
		if !found || !server.Enabled {
			result = errors.Join(result, c.connectionLifecycle.Detach(id))
		}
		if !found {
			c.exposure.removeServer(id)
			events = append(events, c.prepareStatus(ServerStatus{Server: id}, nil))
			continue
		}
		c.exposure.setServer(server)
		status := ServerStatus{Server: server.ID()}
		var operation *activeDial
		if server.Enabled {
			operation, err = c.redialServer(write.ownerCtx, server.ID(), start)
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

// refuseUnrealized records, at the connection status owner, that committed
// sources could not be realized. Withdrawal already retired their previous
// connections; without this refusal an enabled source would read as merely
// disconnected while nothing will dial it. A removed or disabled source has no
// connection to report, so it stays absent instead of becoming a failed ghost.
// The refusal outlives the realization context that just failed.
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
			result = errors.Join(result, c.connectionLifecycle.Refuse(ctx, id, mcpserver.FailureConfiguration))
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
