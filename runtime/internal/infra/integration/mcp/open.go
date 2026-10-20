package mcp

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/flame/runtime/dependency"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	pluginsdk "github.com/Tangerg/flame/runtime/plugin"
	sdkmcp "github.com/Tangerg/go-sdk/mcp"
	"go.opentelemetry.io/otel"
)

var tracer = otel.Tracer("scope/flame/infra/integration/mcp")

// Optional sources must not gate Runtime construction; accepted attempts belong to its lifetime.
func Open(ctx, lifetime context.Context, servers []ServerConfig, oauthSessions OAuthSessionStore,
	configuration func(context.Context, mcpserver.ID) (*Launch, error), callbacks *pluginsdk.Callbacks, client *sdkmcp.Client,
) (result *Connections, err error) {
	if dependency.Missing(ctx) {
		return nil, errors.New("mcp: startup context is required")
	}
	if dependency.Missing(lifetime) {
		return nil, errors.New("mcp: lifetime is required")
	}
	if configuration == nil {
		return nil, errors.New("mcp: connection configuration source is required")
	}
	if client == nil {
		return nil, errors.New("mcp: client is required")
	}
	if oauthSessions != nil && dependency.Missing(oauthSessions) {
		return nil, errors.New("mcp: supplied oauth session store is nil")
	}
	servers = slices.Clone(servers)
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
		servers[index] = srv
	}
	c := &Connections{lifetime: lifetime, callbacks: callbacks, client: client, oauthSessions: oauthSessions}
	if err := c.cancellationErr(ctx); err != nil {
		return nil, err
	}
	defer func() {
		if result == nil {
			err = errors.Join(err, c.Shutdown(context.WithoutCancel(ctx)))
		}
	}()
	for _, srv := range servers {
		attempt, err := c.BeginConnect(lifetime, srv.ID(), configuration)
		if err != nil {
			return nil, err
		}
		attempt.startExecution()
	}
	return c, nil
}
