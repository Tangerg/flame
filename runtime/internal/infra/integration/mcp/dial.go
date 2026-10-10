package mcp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"

	"github.com/Tangerg/flame/runtime/internal/dependency"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// tracer emits the MCP dial / reconnect spans the lower layers don't (per-call
// MCP tool spans come from the mcp module itself). No-op until a provider is
// installed.
var tracer = otel.Tracer("scope/flame/infra/integration/mcp")

// Dial connects to each configured server and discovers its tools. SetToolSink
// publishes the admitted catalog once its consumer is ready.
//
// Failure, two tiers: a config mistake (duplicate name / invalid entry) is
// FATAL (validated before any dial); a reachability failure is TOLERATED
// (recorded "failed" and skipped). An empty config still yields a live,
// initially-empty Connections so runtime configuration can add servers later.
func Dial(
	ctx context.Context,
	lifetime context.Context,
	servers []ServerConfig,
	oauthSessions OAuthSessionStore,
	configuration func(context.Context, mcpserver.ID) (*Launch, error),
) (*Connections, error) {
	if dependency.Missing(ctx) {
		return nil, errors.New("mcp: startup context is required")
	}
	if dependency.Missing(lifetime) {
		return nil, errors.New("mcp: lifetime is required")
	}
	if configuration == nil {
		return nil, errors.New("mcp: connection configuration source is required")
	}
	if oauthSessions != nil && dependency.Missing(oauthSessions) {
		return nil, errors.New("mcp: supplied oauth session store is nil")
	}
	// An empty startup catalog retains a client for later connection admission.
	if len(servers) == 0 {
		return &Connections{
			lifetime: lifetime, client: newClient(), oauthSessions: oauthSessions,
		}, nil
	}
	servers = slices.Clone(servers)
	for i := range servers {
		servers[i] = servers[i].Clone()
	}

	// Validate config before dialing: duplicate names collide tool prefixes and
	// a malformed entry can never work — operator mistakes that should fail
	// loudly at boot, not degrade to a "failed" row.
	seen := make(map[mcpserver.ID]struct{}, len(servers))
	for index := range servers {
		srv := &servers[index]
		if _, dup := seen[srv.ID()]; dup {
			return nil, fmt.Errorf("mcp: duplicate server name %q", srv.ID())
		}
		seen[srv.ID()] = struct{}{}
		if verr := srv.Validate(); verr != nil {
			return nil, fmt.Errorf("mcp: invalid server %q: %w", srv.ID(), verr)
		}
	}

	// Servers share a client identity because no per-server handlers are needed.
	client := newClient()
	c := &Connections{lifetime: lifetime, client: client, oauthSessions: oauthSessions}

	ctx, span := tracer.Start(ctx, "mcp.dial_servers",
		trace.WithAttributes(attribute.Int("mcp.server.count", len(servers))))
	defer span.End()

	toolCount := 0
	failures := 0
	for _, srv := range servers {
		attempt, err := c.BeginConnect(ctx, srv.ID(), configuration)
		if err == nil {
			err = attempt.Run()
		}
		if err != nil {
			slog.ErrorContext(ctx, "mcp: startup connection failed", "server.name", srv.ID().String(), "error", err)
			failures++
		}
	}
	for _, status := range c.Statuses() {
		toolCount += status.ToolCount
	}

	span.SetAttributes(
		attribute.Int("mcp.tool.count", toolCount),
		attribute.Int("mcp.server.failed", failures),
	)
	if failures > 0 {
		span.SetStatus(codes.Error, fmt.Sprintf("%d/%d MCP servers failed to connect", failures, len(servers)))
	}
	return c, nil
}
