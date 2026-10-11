package mcp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"

	"github.com/Tangerg/flame/runtime/internal/infra/integration/managedmcp"

	"github.com/Tangerg/flame/runtime/internal/dependency"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	sdkmcp "github.com/Tangerg/go-sdk/mcp"
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
// The caller registers custom protocol methods on client before dialing;
// every admitted session retains that one client.
// Initialization cancellation rejects the handoff and settles all acquired
// sessions before returning, preserving their retirement diagnostics.
func Dial(
	ctx context.Context,
	lifetime context.Context,
	servers []ServerConfig,
	oauthSessions OAuthSessionStore,
	configuration func(context.Context, mcpserver.ID) (*Launch, error),
	models *managedmcp.Models,
	client *sdkmcp.Client,
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
	c := &Connections{lifetime: lifetime, models: models, client: client, oauthSessions: oauthSessions}
	if err := c.cancellationErr(ctx); err != nil {
		return nil, err
	}
	defer func() {
		if result == nil {
			err = errors.Join(err, c.Shutdown(context.WithoutCancel(ctx)))
		}
	}()

	operationCtx, span := tracer.Start(ctx, "mcp.dial_servers",
		trace.WithAttributes(attribute.Int("mcp.server.count", len(servers))))
	defer span.End()

	toolCount := 0
	failures := 0
	for _, srv := range servers {
		if err := c.cancellationErr(ctx); err != nil {
			return nil, err
		}
		attempt, err := c.BeginConnect(operationCtx, srv.ID(), configuration)
		if err == nil {
			err = attempt.Run()
		}
		if stopped := c.cancellationErr(ctx); stopped != nil {
			return nil, errors.Join(err, stopped)
		}
		if err != nil {
			slog.ErrorContext(operationCtx, "mcp: startup connection failed", "server.name", srv.ID().String(), "error", err)
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
	if err := c.cancellationErr(ctx); err != nil {
		return nil, err
	}
	return c, nil
}
