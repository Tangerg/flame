package mcp

import (
	"context"
	"encoding/json/jsontext"
	"errors"
	"iter"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	mcpapp "github.com/Tangerg/flame/runtime/internal/application/integration/mcp"
	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"

	framecall "github.com/Tangerg/frame/call"
	framevalue "github.com/Tangerg/frame/value"
	sdk "github.com/Tangerg/go-sdk/mcp"
	toolcontract "github.com/Tangerg/scope/core/tool"
	"github.com/samber/lo"
)

const callbackPayloadBytes = 4 << 20

var callbackPayloadLimits = pluginmcp.CallbackLimits{MaxRequestBytes: callbackPayloadBytes, MaxResponseBytes: callbackPayloadBytes}

type Pool struct {
	commands  invocationRestorer
	registry  sourceRegistry
	inner     *connections
	publish   invalidation.Publish
	lifetime  context.Context
	callbacks *pluginmcp.Callbacks
	installed *Installed
}

type invocationRestorer interface {
	RestoreInvocation(context.Context, framecall.Invocation) (context.Context, error)
}

var _ mcpapp.Connections = (*Pool)(nil)

func Open(
	ctx context.Context,
	lifetime context.Context,
	servers []mcpserver.Server,
	oauthSessions oauthSessionStore, registry sourceRegistry,
	callbacks *pluginmcp.Callbacks,
	publish invalidation.Publish,
	commands invocationRestorer, installed *Installed,
) (*Pool, error) {
	if lo.IsNil(registry) || lo.IsNil(commands) || installed == nil || publish == nil {
		return nil, errors.New("mcp connection: source registry, invocation restorer and installed backends are required")
	}
	configs := make([]ServerConfig, len(servers))
	for index, server := range servers {
		configs[index] = ServerConfig{Server: server}
	}
	pool := &Pool{commands: commands, registry: registry, publish: publish, lifetime: lifetime, callbacks: callbacks, installed: installed}
	inner, err := newConnections(ctx, lifetime, configs, oauthSessions, callbacks)
	if err != nil {
		return nil, err
	}
	if err := pluginmcp.RegisterClient(inner.client); err != nil {
		return nil, err
	}
	pool.inner = inner
	if err := inner.start(configs, pool.connectionLaunch); err != nil {
		return nil, err
	}
	return pool, nil
}

func (p *Pool) Statuses() []mcpserver.ConnectionStatus {
	return p.inner.statuses()
}

func (p *Pool) BeginConnect(ctx context.Context, name mcpserver.ID) (mcpapp.Attempt, error) {
	attempt, err := p.inner.beginConnect(ctx, name, p.connectionLaunch)
	if err != nil {
		return nil, err
	}
	return attempt, nil
}

func (p *Pool) BeginAuthorize(ctx context.Context, name mcpserver.ID) (mcpapp.Attempt, error) {
	attempt, err := p.inner.beginAuthorize(ctx, name, p.connectionLaunch)
	if err != nil {
		return nil, err
	}
	return attempt, nil
}

func (p *Pool) Probe(ctx context.Context, server mcpserver.Server) error {
	return p.inner.probe(ctx, ServerConfig{Server: server})
}

func (p *Pool) connectionLaunch(ctx context.Context, name mcpserver.ID) (*launchPermit, error) {
	if _, managed := name.Origin().Installation(); managed {
		value, err := p.installed.acquire(ctx, name)
		if err != nil {
			return nil, err
		}
		server, found, err := p.registry.Dispatchable(ctx, name)
		if err != nil {
			return nil, errors.Join(err, value.close())
		}
		if !found {
			return nil, errors.Join(ErrUnavailable, value.close())
		}
		return &launchPermit{claim: &launchClaim{pending: &launch{config: ServerConfig{Server: server}, managed: value, retire: value.close, admission: admission{Bindings: value.projection.bindings}}}}, nil
	}
	connection, err := p.registry.Connection(ctx, name)
	if err != nil {
		if errors.Is(err, mcpapp.ErrUnknownServer) || errors.Is(err, mcpapp.ErrServerDisabled) {
			err = errors.Join(errSourceWithdrawn, err)
		}
		return nil, err
	}
	return newLaunch(ServerConfig{Server: connection.Server}, nil, nil, admission{})
}

func (p *Pool) Refuse(ctx context.Context, name mcpserver.ID, failure mcpserver.ConnectionFailure) error {
	return p.inner.refuse(ctx, name, failure)
}

func setupContributions(connection mcpapp.Launch) ([]pluginmcp.SetupContribution, error) {
	values := make([]pluginmcp.SetupContribution, 0, len(connection.Contributions))
	for _, contribution := range connection.Contributions {
		value := pluginmcp.SetupContribution{ID: contribution.ID.String(), Export: contribution.Export, Requires: []pluginmcp.SetupDependency{}}
		if contribution.Background {
			installation, found := connection.Server.ID().Origin().Installation()
			if !found {
				return nil, errors.New("mcp: background role requires an installed entry")
			}
			value.Background = &framecall.Target{Scope: extension.ScopeInstallation, Reference: installation.String()}
		}
		for _, required := range connection.Requirements.Snapshot() {
			if required.Contribution != contribution.ID {
				continue
			}
			contract, err := plugincontract.ParseCapability(required.Contract.Bytes())
			if err != nil {
				return nil, err
			}
			dependency := pluginmcp.SetupDependency{ID: required.ID.String(), Contract: contract.Descriptor(), Optional: required.Optional, Bindings: []pluginmcp.SetupBinding{}}
			for _, binding := range required.Bindings {
				dependency.Bindings = append(dependency.Bindings, pluginmcp.SetupBinding{InstallationID: binding.Provider.InstallationID.String(), Digest: binding.Provider.Digest.String(), Contribution: binding.Contribution.String(), Contract: contract.Descriptor()})
			}
			value.Requires = append(value.Requires, dependency)
		}
		values = append(values, value)
	}
	return values, nil
}

func (p *Pool) Detach(name mcpserver.ID) error {
	return p.inner.detach(name)
}

func (p *Pool) SetToolSink(sink func([]toolcontract.Tool, []mcpserver.ID), prepare ToolPreparation) error {
	if sink == nil || prepare == nil {
		return errors.New("mcp: tool sink and preparation are required")
	}
	p.inner.setToolSink(func(catalog []Executable, pending, changed []mcpserver.ID) {
		sink(p.authorizedTools(catalog, prepare), pending)
		if len(changed) > 0 {
			p.publish.Notify(invalidation.ForMCP(changed...))
		}
	})
	return nil
}

func (p *Pool) Shutdown(ctx context.Context) error {
	return p.inner.shutdown(ctx)
}

func connectionError(id mcpserver.ID, err error) error {
	if _, installed := id.Origin().Installation(); installed && errors.Is(err, ErrUnavailable) && !errors.Is(err, plugin.ErrUnavailable) {
		return errors.Join(plugin.ErrUnavailable, err)
	}
	return err
}

func available(server mcpserver.Server, found bool, err error) (mcpserver.Server, error) {
	if err == nil && (!found || !server.Enabled) {
		err = ErrUnavailable
	}
	return server, err
}

func (p *Pool) BorrowSession(ctx context.Context, id mcpserver.ID) (_ *BorrowedSession, err error) {
	defer func() { err = connectionError(id, err) }()
	descriptor, err := available(p.registry.Dispatchable(ctx, id))
	if err != nil {
		return nil, err
	}
	session, err := p.inner.waitSession(ctx, descriptor)
	if err != nil {
		return nil, err
	}
	return p.inner.borrowSession(ctx, session)
}

func (p *Pool) InstallationActivation(ctx context.Context, id mcpserver.ID) (_ plugin.Activation, err error) {
	defer func() { err = connectionError(id, err) }()
	descriptor, err := available(p.registry.Definition(ctx, id))
	if err != nil {
		return plugin.Activation{}, err
	}
	session, err := p.inner.serviceSession(ctx, descriptor)
	if err != nil {
		return plugin.Activation{}, err
	}
	return p.inner.activation(session)
}

func (p *Pool) AwaitInstallationActivation(ctx context.Context, id mcpserver.ID) (_ plugin.Activation, err error) {
	defer func() { err = connectionError(id, err) }()
	descriptor, err := available(p.registry.Definition(ctx, id))
	if err != nil {
		return plugin.Activation{}, err
	}
	session, err := p.inner.waitSession(ctx, descriptor)
	if err != nil {
		return plugin.Activation{}, err
	}
	return p.inner.activation(session)
}

func (p *Pool) BorrowBoundSession(ctx context.Context, id mcpserver.ID, authority framevalue.Digest) (_ *BorrowedSession, err error) {
	defer func() { err = connectionError(id, err) }()
	descriptor, err := available(p.registry.Dispatchable(ctx, id))
	if err != nil {
		return nil, err
	}
	if descriptor.AuthorityFingerprint() != authority {
		return nil, ErrUnavailable
	}
	session, err := p.inner.waitSession(ctx, descriptor)
	if err != nil {
		return nil, err
	}
	return p.inner.borrowSession(ctx, session)
}

func (p *Pool) SessionActor(ctx context.Context, id mcpserver.ID, expected *sdk.ClientSession) (_ operation.Actor, err error) {
	defer func() { err = connectionError(id, err) }()
	borrowed, _ := ctx.Value(borrowContextKey{}).(*BorrowedSession)
	if borrowed == nil || borrowed.managed == nil || borrowed.session != expected || borrowed.ctx.Err() != nil || ctx.Err() != nil {
		return operation.Actor{}, ErrUnavailable
	}
	actor := borrowed.managed.actor()
	installation, managed := id.Origin().Installation()
	if !managed || actor.InstallationID() != installation || actor.Server() != id.Name() {
		return operation.Actor{}, ErrUnavailable
	}
	return actor, nil
}

func (p *Pool) OpenRPC(ctx context.Context, id mcpserver.ID, session *sdk.ClientSession, callbacks ...pluginmcp.Callback) (sdk.ParamsBase, func(), error) {
	actor, err := p.SessionActor(ctx, id, session)
	if err != nil {
		return sdk.ParamsBase{}, nil, err
	}
	ref, release, err := p.callbacks.OpenRPC(operation.WithActor(ctx, actor), session, callbackPayloadLimits, callbacks...)
	if err != nil {
		return sdk.ParamsBase{}, nil, err
	}
	metadata, err := pluginmcp.CallbackMetadata(ref)
	if err != nil {
		release()
		return sdk.ParamsBase{}, nil, err
	}
	return metadata, release, nil
}

func (p *Pool) toolHost(executable Executable) ToolHost {
	return func(ctx context.Context) (sdk.ParamsBase, func() error, error) {
		borrowed, err := executable.BorrowSession(ctx)
		if err != nil {
			return sdk.ParamsBase{}, nil, err
		}
		metadata, release, err := p.OpenRPC(borrowed.Context(), executable.SourceConfig().ID(), borrowed.Session())
		if err != nil {
			return sdk.ParamsBase{}, nil, errors.Join(err, borrowed.Close())
		}
		return metadata, func() error { release(); return borrowed.Close() }, nil
	}
}

func (p *Pool) Stream(ctx context.Context, id mcpserver.ID, session *sdk.ClientSession, binding *pluginmcp.Binding, command string, input framevalue.JSON, metadata sdk.ParamsBase) iter.Seq2[pluginmcp.StreamValue[jsontext.Value, jsontext.Value], error] {
	return func(yield func(pluginmcp.StreamValue[jsontext.Value, jsontext.Value], error) bool) {
		if _, err := p.SessionActor(ctx, id, session); err != nil {
			yield(pluginmcp.StreamValue[jsontext.Value, jsontext.Value]{}, err)
			return
		}
		for value, err := range binding.StreamWithMetadata(ctx, p.callbacks, callbackPayloadLimits, pluginmcp.CommandID(command), input, metadata) {
			if !yield(value, err) {
				return
			}
		}
	}
}

type ToolCall func(toolcontract.Invocation) (toolcontract.Result, error)

type ToolHost func(context.Context) (sdk.ParamsBase, func() error, error)

// On success the caller owns release even if final authority admission refuses
// dispatch; on failure preparation releases what it acquired.
type ToolPreparation func(context.Context, Executable, ToolHost) (ToolCall, func() error, error)

func (p *Pool) OpenCallbacks(ctx context.Context, id mcpserver.ID, session *sdk.ClientSession, callbacks ...pluginmcp.Callback) (sdk.ParamsBase, func(), error) {
	if _, err := p.SessionActor(ctx, id, session); err != nil {
		return sdk.ParamsBase{}, nil, err
	}
	ref, closeGrant, err := p.callbacks.Open(ctx, session, callbackPayloadLimits, callbacks...)
	if err != nil {
		return sdk.ParamsBase{}, nil, err
	}
	metadata, err := pluginmcp.CallbackMetadata(ref)
	if err != nil {
		closeGrant()
		return sdk.ParamsBase{}, nil, err
	}
	return metadata, closeGrant, nil
}
