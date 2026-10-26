package mcp

import (
	"context"
	"errors"
	"fmt"

	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	sdkmcp "github.com/Tangerg/go-sdk/mcp"
	"github.com/samber/lo"
	"go.opentelemetry.io/otel"
)

var tracer = otel.Tracer("scope/flame/adapter/integration/mcp")

// newConnections validates the complete ledger before any owned attempt starts.
func newConnections(ctx, lifetime context.Context, servers []ServerConfig, oauthSessions oauthSessionStore,
	callbacks *pluginsdk.Callbacks, client *sdkmcp.Client,
) (*connections, error) {
	if lo.IsNil(ctx) {
		return nil, errors.New("mcp: startup context is required")
	}
	if lo.IsNil(lifetime) {
		return nil, errors.New("mcp: lifetime is required")
	}
	if client == nil {
		return nil, errors.New("mcp: client is required")
	}
	if oauthSessions != nil && lo.IsNil(oauthSessions) {
		return nil, errors.New("mcp: supplied oauth session store is nil")
	}
	seen := make(map[mcpserver.ID]struct{}, len(servers))
	for index := range servers {
		srv := servers[index].Clone()
		if _, duplicate := seen[srv.ID()]; duplicate {
			return nil, fmt.Errorf("mcp: duplicate server name %q", srv.ID())
		}
		seen[srv.ID()] = struct{}{}
		if err := srv.Validate(); err != nil {
			return nil, fmt.Errorf("mcp: invalid server %q: %w", srv.ID(), err)
		}
	}
	c := &connections{lifetime: lifetime, callbacks: callbacks, client: client, oauthSessions: oauthSessions}
	if err := c.cancellationErr(ctx); err != nil {
		return nil, err
	}
	return c, nil
}

// All initial admissions exist before any configuration I/O. A collaborator's
// immediate handshake can therefore join its already owned dependency attempt.
func (c *connections) start(servers []ServerConfig, configuration func(context.Context, mcpserver.ID) (*launchPermit, error)) (err error) {
	defer func() {
		if err != nil {
			err = errors.Join(err, c.shutdown(context.WithoutCancel(c.lifetime)))
		}
	}()
	if configuration == nil {
		return errors.New("mcp: connection configuration source is required")
	}
	attempts := make([]*attempt, 0, len(servers))
	for _, server := range servers {
		attempt, err := c.beginConnect(c.lifetime, server.ID(), configuration)
		if err != nil {
			return err
		}
		attempts = append(attempts, attempt)
	}
	for _, attempt := range attempts {
		attempt.startExecution()
	}
	return nil
}
