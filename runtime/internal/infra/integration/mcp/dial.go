package mcp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"

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

// Dial connects to each configured server, lists its tools, and returns the
// Connections handle alongside the merged model-facing tool list. The server
// name namespaces tools across servers.
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
) (*Connections, []Executable, error) {
	if ctx == nil {
		return nil, nil, errors.New("mcp: startup context is required")
	}
	if lifetime == nil {
		return nil, nil, errors.New("mcp: lifetime is required")
	}
	if configuration == nil {
		return nil, nil, errors.New("mcp: connection configuration source is required")
	}
	// Always carry a client, even with zero servers: the registry starts empty
	// and the common path is a 0-server boot followed by a runtime Configure,
	// which re-dials with this client.
	if len(servers) == 0 {
		return &Connections{
			lifetime: lifetime, client: newClient(), oauthSessions: oauthSessions,
		}, nil, nil
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
			return nil, nil, fmt.Errorf("mcp: duplicate server name %q", srv.ID())
		}
		seen[srv.ID()] = struct{}{}
		if verr := srv.Validate(); verr != nil {
			return nil, nil, fmt.Errorf("mcp: invalid server %q: %w", srv.ID(), verr)
		}
	}

	// One client identity for every server — none of flame's connections need
	// per-server handlers (sampling / list-changed), so they share it. Retained
	// so Reconnect / Configure can re-dial with it.
	client := newClient()
	c := &Connections{lifetime: lifetime, client: client, oauthSessions: oauthSessions}

	ctx, span := tracer.Start(ctx, "mcp.dial_servers",
		trace.WithAttributes(attribute.Int("mcp.server.count", len(servers))))
	defer span.End()

	var tools []Executable
	failures := 0
	for _, srv := range servers {
		configuredServer := &server{id: srv.ID(), config: srv, oauth: srv.OAuthHandler}
		configuredServer.config.OAuthHandler = nil
		input, err := configuration(ctx, srv.ID())
		var prepared *launch
		var current ServerConfig
		if err == nil {
			prepared, err = input.take()
			if err == nil {
				current = prepared.config
			}
		} else {
			err = errors.Join(err, input.Close())
		}
		if err == nil && current.ID() != srv.ID() {
			err = errors.New("mcp: connection configuration source changed identity")
		}
		if err == nil && current.Transport == TransportHTTP && current.OAuthHandler == nil && current.Authorization == "" {
			current.OAuthHandler, err = restoreOAuthHandler(ctx, lifetime, oauthSessions, current.oauthTarget())
		}
		if err != nil {
			if prepared != nil {
				err = errors.Join(err, prepared.close())
			}
			slog.ErrorContext(ctx, "mcp: startup admission failed", "server.name", srv.ID().String(), "error", err)
			configuredServer.settle(nil, nil, mcpserver.ConnectionFailed, mcpserver.FailureConfiguration)
			failures++
			c.servers = append(c.servers, configuredServer)
			continue
		}
		srv = current.Clone()
		configuredServer.config = srv.Clone()
		configuredServer.config.OAuthHandler = nil
		configuredServer.oauth = srv.OAuthHandler
		prepared.config = current
		session, cleanupSession, derr := dial(ctx, lifetime, client, prepared)
		if derr != nil {
			slog.ErrorContext(ctx, "mcp: startup connection failed",
				"server.name", srv.ID().String(), "error", derr,
			)
			state, failure := failedStatus(derr, mcpserver.FailureConnection)
			configuredServer.settle(nil, nil, state, failure)
			if state == mcpserver.ConnectionNeedsAuth {
				configuredServer.oauth = nil
			}
			failures++
			c.servers = append(c.servers, configuredServer)
			continue
		}
		c.ownSessionLocked(session, cleanupSession)
		srcTools, terr := sourceTools(ctx, c, srv, session)
		if terr != nil {
			// A session that cannot produce a valid tool catalog is unusable.
			// Preserve a close failure in diagnostics as well as the primary cause;
			// boot deliberately degrades this one server to failed rather than
			// aborting every independent MCP connection.
			cause := errors.Join(terr, c.closeSession(ctx, session))
			slog.ErrorContext(ctx, "mcp: startup tool discovery failed",
				"server.name", srv.ID().String(), "error", cause,
			)
			state, failure := failedStatus(terr, mcpserver.FailureToolDiscovery)
			configuredServer.settle(nil, nil, state, failure)
			failures++
			c.servers = append(c.servers, configuredServer)
			continue
		}
		configuredServer.settle(session, srcTools, mcpserver.ConnectionConnected, "")
		tools = append(tools, srcTools...)
		c.servers = append(c.servers, configuredServer)
	}

	span.SetAttributes(
		attribute.Int("mcp.tool.count", len(tools)),
		attribute.Int("mcp.server.failed", failures),
	)
	if failures > 0 {
		span.SetStatus(codes.Error, fmt.Sprintf("%d/%d MCP servers failed to connect", failures, len(servers)))
	}
	return c, tools, nil
}
