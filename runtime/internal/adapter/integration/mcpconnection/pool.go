// Package mcpconnection adapts persisted MCP server definitions to the live
// MCP connection pool. It is the only runtime layer that knows both the domain
// registry shape and the infrastructure dial shape.
package mcpconnection

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"

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

var (
	_ mcpapp.StatusReader        = (*Pool)(nil)
	_ mcpapp.ConnectionControl   = (*Pool)(nil)
	_ mcpapp.ConnectionLifecycle = (*Pool)(nil)
)

// Open establishes the enabled MCP connections present at runtime startup.
// Unreachable but valid servers remain in the pool as failed, matching the
// infrastructure pool's normal boot semantics.
func Open(
	ctx context.Context,
	lifetime context.Context,
	servers []mcpserver.Server,
	oauthSessions mcp.OAuthSessionStore, registry sourceRegistry,
) (*Pool, []toolcontract.Tool, error) {
	if registry == nil {
		return nil, nil, errors.New("mcp connection: source registry is required")
	}
	configs, err := configsFromServers(servers)
	if err != nil {
		return nil, nil, err
	}
	pool := &Pool{registry: registry}
	inner, toolset, err := mcp.Dial(ctx, lifetime, configs, oauthSessions, pool.connectionConfig)
	if err != nil {
		return nil, nil, err
	}
	pool.inner = inner
	return pool, pool.authorizedTools(toolset), nil
}

func (p *Pool) Statuses() []mcpserver.ConnectionStatus {
	return p.inner.Statuses()
}

func (p *Pool) Reconnect(ctx context.Context, name mcpserver.ID) error {
	config, err := p.admittedConfig(ctx, name)
	if err != nil {
		return err
	}
	return p.inner.Configure(ctx, config)
}

func (p *Pool) Authorize(ctx context.Context, name mcpserver.ID) error {
	config, err := p.admittedConfig(ctx, name)
	if err != nil {
		return err
	}
	return p.inner.Authorize(ctx, config)
}

func (p *Pool) Probe(ctx context.Context, server mcpserver.Server) error {
	cfg, err := configFromServer(server)
	if err != nil {
		return err
	}
	return p.inner.Probe(ctx, cfg)
}

func (p *Pool) Configure(ctx context.Context, name mcpserver.ID) error {
	cfg, err := p.admittedConfig(ctx, name)
	if err != nil {
		return err
	}
	return p.inner.Configure(ctx, cfg)
}

// admittedConfig obtains the configuration a new connection must use. When the
// source refuses, the refusal becomes the live state: a removed or disabled
// source is detached, any other refusal settles as a configuration failure,
// and in both cases the previous session stops serving tools.
func (p *Pool) admittedConfig(ctx context.Context, name mcpserver.ID) (mcp.ServerConfig, error) {
	cfg, err := p.connectionConfig(ctx, name)
	if err == nil {
		return cfg, nil
	}
	if cause := context.Cause(ctx); cause != nil {
		return mcp.ServerConfig{}, errors.Join(err, cause)
	}
	if errors.Is(err, mcpapp.ErrUnknownServer) || errors.Is(err, mcpapp.ErrServerDisabled) {
		return mcp.ServerConfig{}, errors.Join(err, p.inner.Detach(name))
	}
	return mcp.ServerConfig{}, errors.Join(err, p.inner.Refuse(ctx, name, mcpserver.FailureConfiguration))
}

func (p *Pool) connectionConfig(ctx context.Context, name mcpserver.ID) (mcp.ServerConfig, error) {
	server, err := p.registry.Connection(ctx, name)
	if err != nil {
		return mcp.ServerConfig{}, err
	}
	return configFromServer(server)
}

func (p *Pool) Refuse(ctx context.Context, name mcpserver.ID, failure mcpserver.ConnectionFailure) error {
	return p.inner.Refuse(ctx, name, failure)
}

func (p *Pool) Detach(name mcpserver.ID) error {
	return p.inner.Detach(name)
}

// SetToolSink wires live connection changes to the resolver's atomically
// replaceable MCP tool catalog.
func (p *Pool) SetToolSink(sink func([]toolcontract.Tool)) {
	p.inner.SetToolSink(func(catalog []mcp.Executable) { sink(p.authorizedTools(catalog)) })
}

// Shutdown releases every live connection under the caller's shutdown budget.
func (p *Pool) Shutdown(ctx context.Context) error {
	return p.inner.Shutdown(ctx)
}

func configsFromServers(servers []mcpserver.Server) ([]mcp.ServerConfig, error) {
	if len(servers) == 0 {
		return nil, nil
	}
	out := make([]mcp.ServerConfig, len(servers))
	for i, server := range servers {
		cfg, err := configFromServer(server)
		if err != nil {
			return nil, fmt.Errorf("mcp connection: map server %q: %w", server.ID(), err)
		}
		out[i] = cfg
	}
	return out, nil
}

func configFromServer(server mcpserver.Server) (mcp.ServerConfig, error) {
	if err := server.Validate(); err != nil {
		return mcp.ServerConfig{}, fmt.Errorf("validate domain server: %w", err)
	}
	transport, err := transportFromDomain(server.Transport)
	if err != nil {
		return mcp.ServerConfig{}, err
	}
	cfg := mcp.ServerConfig{
		SourceFingerprint: server.AuthorityFingerprint(),
		Source:            server.Source,
		Name:              server.Name,
		Transport:         transport,
	}
	if timeout, bounded := server.HandshakeTimeout.Duration(); bounded {
		cfg.HandshakeTimeout = &timeout
	}
	switch server.Transport {
	case mcpserver.TransportStreamableHTTP:
		cfg.Endpoint = server.URL
		cfg.Authorization = server.Authorization
		cfg.Headers = maps.Clone(server.Headers)
	case mcpserver.TransportStdio:
		cfg.Command = server.Command
		cfg.Args = slices.Clone(server.Args)
		cfg.Env = flattenEnv(server.Env)
		cfg.Dir = server.Dir
	}
	if err := cfg.Validate(); err != nil {
		return mcp.ServerConfig{}, fmt.Errorf("validate runtime config: %w", err)
	}
	return cfg, nil
}

func transportFromDomain(transport mcpserver.Transport) (mcp.Transport, error) {
	switch transport {
	case mcpserver.TransportStreamableHTTP:
		return mcp.TransportHTTP, nil
	case mcpserver.TransportStdio:
		return mcp.TransportStdio, nil
	default:
		return "", fmt.Errorf("unknown domain transport %q", transport)
	}
}

func flattenEnv(values map[string]string) []string {
	if len(values) == 0 {
		return nil
	}
	entries := make([]string, 0, len(values))
	for key, value := range values {
		entries = append(entries, key+"="+value)
	}
	slices.Sort(entries)
	return entries
}
