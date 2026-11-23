package mcp

import (
	"context"
	"errors"
	"fmt"

	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/samber/lo"
	"go.opentelemetry.io/otel"
)

var tracer = otel.Tracer("github.com/Tangerg/flame/runtime/internal/adapter/integration/mcp")

func newConnections(ctx, lifetime context.Context, servers []ServerConfig, oauthSessions oauthSessionStore,
	callbacks *pluginmcp.Callbacks,
) (*connections, error) {
	if lo.IsNil(ctx) {
		return nil, errors.New("mcp: startup context is required")
	}
	if lo.IsNil(lifetime) {
		return nil, errors.New("mcp: lifetime is required")
	}
	if lo.IsNil(oauthSessions) || callbacks == nil {
		return nil, errors.New("mcp: oauth session store and Runtime callbacks are required")
	}
	seen := make(map[mcpserver.ID]struct{}, len(servers))
	for _, srv := range servers {
		if _, duplicate := seen[srv.ID()]; duplicate {
			return nil, fmt.Errorf("mcp: duplicate server name %q", srv.ID())
		}
		seen[srv.ID()] = struct{}{}
		if err := srv.Validate(); err != nil {
			return nil, fmt.Errorf("mcp: invalid server %q: %w", srv.ID(), err)
		}
	}
	c := &connections{lifetime: lifetime, callbacks: callbacks, oauthSessions: oauthSessions,
		sessions: make(map[*sdk.ClientSession]*ownedSession)}
	c.client = newClient(c.toolListChanged)
	if err := c.cancellationErr(ctx); err != nil {
		return nil, err
	}
	return c, nil
}

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
