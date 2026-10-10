// Package mcpconnection adapts persisted MCP server definitions to the live
// MCP connection pool. It is the only runtime layer that knows both the domain
// registry shape and the infrastructure dial shape.
package mcpconnection

import (
	"context"
	"errors"

	mcpapp "github.com/Tangerg/flame/runtime/internal/application/integration/mcp"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/infra/integration/mcp"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

// Pool owns connection lifetime. Every new connection obtains its configuration
// from the source owner; retained configurations describe only existing sessions.
type Pool struct {
	registry sourceRegistry
	inner    *mcp.Connections
}

var _ mcpapp.Connections = (*Pool)(nil)

// Open establishes the enabled MCP connections present at runtime startup.
// Unreachable but valid servers remain in the pool as failed, matching the
// infrastructure pool's normal boot semantics.
func Open(
	ctx context.Context,
	lifetime context.Context,
	servers []mcpserver.Server,
	oauthSessions mcp.OAuthSessionStore, registry sourceRegistry,
) (*Pool, error) {
	if registry == nil {
		return nil, errors.New("mcp connection: source registry is required")
	}
	configs := make([]mcp.ServerConfig, len(servers))
	for index, server := range servers {
		configs[index] = mcp.ServerConfig{Server: server.Clone()}
	}
	pool := &Pool{registry: registry}
	inner, err := mcp.Dial(ctx, lifetime, configs, oauthSessions, pool.connectionLaunch)
	if err != nil {
		return nil, err
	}
	pool.inner = inner
	return pool, nil
}

func (p *Pool) Statuses() []mcpserver.ConnectionStatus {
	return p.inner.Statuses()
}

func (p *Pool) BeginConnect(ctx context.Context, name mcpserver.ID) (mcpapp.Attempt, error) {
	attempt, err := p.inner.BeginConnect(ctx, name, p.connectionLaunch)
	if err != nil {
		return nil, err
	}
	return attempt, nil
}

func (p *Pool) BeginAuthorize(ctx context.Context, name mcpserver.ID) (mcpapp.Attempt, error) {
	attempt, err := p.inner.BeginAuthorize(ctx, name, p.connectionLaunch)
	if err != nil {
		return nil, err
	}
	return attempt, nil
}

func (p *Pool) Probe(ctx context.Context, server mcpserver.Server) error {
	return p.inner.Probe(ctx, mcp.ServerConfig{Server: server.Clone()})
}

func (p *Pool) connectionLaunch(ctx context.Context, name mcpserver.ID) (*mcp.Launch, error) {
	connection, err := p.registry.Connection(ctx, name)
	if err != nil {
		if errors.Is(err, mcpapp.ErrUnknownServer) || errors.Is(err, mcpapp.ErrServerDisabled) {
			err = errors.Join(mcp.ErrSourceWithdrawn, err)
		}
		return nil, err
	}
	var retire func() error
	if connection.Retire != nil {
		retire = connection.Retire.Close
	}
	var stdio *mcp.Stdio
	if connection.Stdio != nil {
		executable := connection.Stdio
		stdio = &mcp.Stdio{Command: executable.Command, Args: executable.Args, Env: executable.Env, Dir: executable.Dir}
	}
	return mcp.NewLaunch(mcp.ServerConfig{Server: connection.Server}, stdio, retire)
}

func (p *Pool) Refuse(ctx context.Context, name mcpserver.ID, failure mcpserver.ConnectionFailure) error {
	return p.inner.Refuse(ctx, name, failure)
}

func (p *Pool) Detach(name mcpserver.ID) error {
	return p.inner.Detach(name)
}

// SetToolSink publishes the initial catalog and live connection changes to the
// resolver through the connection owner's publication boundary.
func (p *Pool) SetToolSink(sink func([]toolcontract.Tool)) {
	p.inner.SetToolSink(func(catalog []mcp.Executable) { sink(p.authorizedTools(catalog)) })
}

// Shutdown releases every live connection under the caller's shutdown budget.
func (p *Pool) Shutdown(ctx context.Context) error {
	return p.inner.Shutdown(ctx)
}
