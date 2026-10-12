// Package mcpconnection adapts persisted MCP server definitions to the live
// MCP connection pool. It is the only runtime layer that knows both the domain
// registry shape and the infrastructure dial shape.
package mcpconnection

import (
	"context"
	"errors"

	"github.com/Tangerg/flame/runtime/internal/infra/integration/managedmcp"
	sdk "github.com/Tangerg/go-sdk/mcp"

	mcpapp "github.com/Tangerg/flame/runtime/internal/application/integration/mcp"
	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/dependency"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/infra/integration/mcp"
	memoryrpc "github.com/Tangerg/flame/runtime/internal/plugins/memory/rpc"
	schedulerpc "github.com/Tangerg/flame/runtime/internal/plugins/schedules/rpc"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

// Pool owns connection lifetime. Every new connection obtains its configuration
// from the source owner; retained configurations describe only existing sessions.
type Pool struct {
	registry      sourceRegistry
	inner         *mcp.Connections
	memoryContext MemoryContext
	publish       invalidation.Publish
}

var _ mcpapp.Connections = (*Pool)(nil)

// Startup connection waits belong to the pool, so one optional source cannot block Runtime management.
func Open(
	ctx context.Context,
	lifetime context.Context,
	servers []mcpserver.Server,
	oauthSessions mcp.OAuthSessionStore, registry sourceRegistry,
	models *managedmcp.Models,
	memoryContext MemoryContext,
	publish invalidation.Publish,
) (*Pool, error) {
	if dependency.Missing(registry) {
		return nil, errors.New("mcp connection: source registry is required")
	}
	configs := make([]mcp.ServerConfig, len(servers))
	for index, server := range servers {
		configs[index] = mcp.ServerConfig{Server: server.Clone()}
	}
	pool := &Pool{registry: registry, memoryContext: memoryContext, publish: publish}
	client := mcp.NewClient()
	if err := errors.Join(memoryrpc.RegisterClient(client), schedulerpc.RegisterClient(client)); err != nil {
		return nil, err
	}
	inner, err := mcp.Open(ctx, lifetime, configs, oauthSessions, pool.connectionLaunch, models, client)
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
func (p *Pool) SetToolSink(sink func([]toolcontract.Tool, []mcpserver.ID)) {
	p.inner.SetToolSink(func(catalog []mcp.Executable, pending, changed []mcpserver.ID) {
		sink(p.authorizedTools(catalog), pending)
		if len(changed) > 0 {
			p.publish.Notify(invalidation.ForMCP(changed...))
		}
	})
}

// Shutdown releases every live connection under the caller's shutdown budget.
func (p *Pool) Shutdown(ctx context.Context) error {
	return p.inner.Shutdown(ctx)
}

func (p *Pool) ServiceSession(ctx context.Context, id mcpserver.ID) (*sdk.ClientSession, error) {
	descriptor, found, err := p.registry.Dispatchable(ctx, id)
	if err != nil {
		return nil, err
	}
	if !found || !descriptor.Enabled {
		return nil, mcp.ErrUnavailable
	}
	return p.inner.ServiceSession(ctx, descriptor)
}

type MemoryContext func(context.Context, mcp.Executable) (context.Context, func(), error)
