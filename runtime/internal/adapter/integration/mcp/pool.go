package mcp

import (
	"context"
	"encoding/json/jsontext"
	"errors"
	"iter"

	"github.com/Tangerg/flame/packages/go-sdk/extension"

	flameidentity "github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/qimu/jsonvalue"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/qimu/portable"
	mcpapp "github.com/Tangerg/flame/runtime/internal/application/integration/mcp"
	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	sdk "github.com/Tangerg/go-sdk/mcp"
	toolcontract "github.com/Tangerg/scope/core/tool"
	"github.com/samber/lo"
)

// callbackPayloadLimits caps a plugin callback's request and response bodies at 4 MiB.
var callbackPayloadLimits = qimumcp.CallbackLimits{MaxRequestBytes: 4 << 20, MaxResponseBytes: 4 << 20}

// Pool owns connection lifetime. Every new connection obtains its configuration
// from the source owner; retained configurations describe only existing sessions.
type Pool struct {
	commands  invocationRestorer
	registry  sourceRegistry
	inner     *connections
	publish   invalidation.Publish
	lifetime  context.Context
	callbacks *qimumcp.Callbacks
	installed *Installed
}

type invocationRestorer interface {
	RestoreInvocation(context.Context, portable.Invocation) (context.Context, error)
}

var _ mcpapp.Connections = (*Pool)(nil)

// Startup connection waits belong to the pool, so one optional source cannot block Runtime management.
func Open(
	ctx context.Context,
	lifetime context.Context,
	servers []mcpserver.Server,
	oauthSessions oauthSessionStore, registry sourceRegistry,
	callbacks *qimumcp.Callbacks,
	publish invalidation.Publish,
	commands invocationRestorer, installed *Installed,
) (*Pool, error) {
	if lo.IsNil(registry) {
		return nil, errors.New("mcp connection: source registry is required")
	}
	configs := make([]ServerConfig, len(servers))
	for index, server := range servers {
		configs[index] = ServerConfig{Server: server.Clone()}
	}
	pool := &Pool{commands: commands, registry: registry, publish: publish, lifetime: lifetime, callbacks: callbacks, installed: installed}
	client := newClient()
	if err := qimumcp.RegisterClient(client); err != nil {
		return nil, err
	}
	inner, err := newConnections(ctx, lifetime, configs, oauthSessions, callbacks, client)
	if err != nil {
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
	return p.inner.probe(ctx, ServerConfig{Server: server.Clone()})
}

func (p *Pool) connectionLaunch(ctx context.Context, name mcpserver.ID) (*launchPermit, error) {
	if _, managed := name.Origin().Installation(); managed {
		if p.installed == nil {
			return nil, errors.New("mcp: installed backend requires Qimu")
		}
		value, err := p.installed.acquire(ctx, name)
		if err != nil {
			return nil, err
		}
		server, found, err := p.registry.Dispatchable(ctx, name)
		if err != nil || !found {
			return nil, errors.Join(ErrUnavailable, err, value.close())
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

func setupContributions(connection mcpapp.Launch) ([]qimumcp.SetupContribution, error) {
	values := make([]qimumcp.SetupContribution, 0, len(connection.Contributions))
	for _, contribution := range connection.Contributions {
		value := qimumcp.SetupContribution{ID: contribution.ID.String(), Export: contribution.Export, Requires: []qimumcp.SetupDependency{}}
		if contribution.Background {
			installation, found := connection.Server.ID().Origin().Installation()
			if !found {
				return nil, errors.New("mcp: background role requires an installed entry")
			}
			value.Background = &portable.Target{Scope: extension.ScopeInstallation, Reference: installation.String()}
		}
		for _, required := range connection.Requirements.Snapshot() {
			if required.Contribution != contribution.ID {
				continue
			}
			contract, err := portable.ParseCapability(required.Contract.JSON())
			if err != nil {
				return nil, err
			}
			dependency := qimumcp.SetupDependency{ID: required.ID.String(), Contract: contract.Descriptor(), Optional: required.Optional, Bindings: []qimumcp.SetupBinding{}}
			for _, binding := range required.Bindings {
				dependency.Bindings = append(dependency.Bindings, qimumcp.SetupBinding{InstallationID: binding.Provider.InstallationID.String(), Digest: binding.Provider.Digest.String(), Contribution: binding.Contribution.String(), Contract: contract.Descriptor()})
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

// setToolSink publishes the initial catalog and live connection changes to the
// resolver through the connection owner's publication boundary.
func (p *Pool) SetToolSink(sink func([]toolcontract.Tool, []mcpserver.ID), prepare ToolPreparation) {
	p.inner.setToolSink(func(catalog []Executable, pending, changed []mcpserver.ID) {
		sink(p.authorizedTools(catalog, prepare), pending)
		if len(changed) > 0 {
			p.publish.Notify(invalidation.ForMCP(changed...))
		}
	})
}

// Shutdown releases every live connection under the caller's shutdown budget.
func (p *Pool) Shutdown(ctx context.Context) error {
	return p.inner.shutdown(ctx)
}

func connectionError(id mcpserver.ID, err error) error {
	if _, installed := id.Origin().Installation(); installed && errors.Is(err, ErrUnavailable) && !errors.Is(err, plugin.ErrUnavailable) {
		return errors.Join(plugin.ErrUnavailable, err)
	}
	return err
}

func (p *Pool) BorrowSession(ctx context.Context, id mcpserver.ID) (_ *BorrowedSession, err error) {
	defer func() { err = connectionError(id, err) }()
	descriptor, found, err := p.registry.Dispatchable(ctx, id)
	if err != nil {
		return nil, err
	}
	if !found || !descriptor.Enabled {
		return nil, ErrUnavailable
	}
	session, err := p.inner.serviceSession(ctx, descriptor)
	if err != nil {
		return nil, err
	}
	return p.inner.borrowSession(ctx, session)
}

func (p *Pool) InstallationActivation(ctx context.Context, id mcpserver.ID) (_ plugin.Activation, err error) {
	defer func() { err = connectionError(id, err) }()
	descriptor, found, err := p.registry.Definition(ctx, id)
	if err != nil {
		return plugin.Activation{}, errors.Join(plugin.ErrUnavailable, err)
	}
	if !found || !descriptor.Enabled {
		return plugin.Activation{}, ErrUnavailable
	}
	session, err := p.inner.serviceSession(ctx, descriptor)
	if err != nil {
		return plugin.Activation{}, err
	}
	// Capturing a graph is a read of the catalog's original observation. Its
	// new business owner validates and retains it under the shared admission;
	// this projection must not acquire a nested execution Use.
	return p.inner.activation(session)
}

// AwaitInstallationActivation joins the source's already owned connection
// attempt. Callers wait outside Host admission, then retain and validate this
// exact projection under admission; waiting never starts a replacement attempt.
func (p *Pool) AwaitInstallationActivation(ctx context.Context, id mcpserver.ID) (_ plugin.Activation, err error) {
	defer func() { err = connectionError(id, err) }()
	descriptor, found, err := p.registry.Definition(ctx, id)
	if err != nil {
		return plugin.Activation{}, errors.Join(plugin.ErrUnavailable, err)
	}
	if !found || !descriptor.Enabled {
		return plugin.Activation{}, ErrUnavailable
	}
	session, err := p.inner.waitSession(ctx, descriptor)
	if err != nil {
		return plugin.Activation{}, err
	}
	return p.inner.activation(session)
}

func (p *Pool) BorrowBoundSession(ctx context.Context, id mcpserver.ID, authority flameidentity.Digest) (_ *BorrowedSession, err error) {
	defer func() { err = connectionError(id, err) }()
	descriptor, found, err := p.registry.Dispatchable(ctx, id)
	if err != nil {
		return nil, err
	}
	if !found || !descriptor.Enabled || descriptor.AuthorityFingerprint() != authority {
		return nil, ErrUnavailable
	}
	session, err := p.inner.waitSession(ctx, descriptor)
	if err != nil {
		return nil, err
	}
	return p.inner.borrowSession(ctx, session)
}

// SessionActor captures only the registry source realized by this exact live
// session. The Run transaction rechecks its immutable authority before commit.
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

// OpenRPC preserves the invoking call while applying this backend's frozen
// dependency authority. Retirement is owned by the same session ledger.
func (p *Pool) OpenRPC(ctx context.Context, id mcpserver.ID, session *sdk.ClientSession, callbacks ...qimumcp.Callback) (sdk.ParamsBase, func(), error) {
	if p.callbacks == nil {
		return sdk.ParamsBase{}, nil, errors.New("mcp: Runtime callbacks are unavailable")
	}
	actor, err := p.SessionActor(ctx, id, session)
	if err != nil {
		return sdk.ParamsBase{}, nil, err
	}
	ref, release, err := p.callbacks.OpenRPC(operation.WithActor(ctx, actor), session, callbackPayloadLimits, callbacks...)
	if err != nil {
		return sdk.ParamsBase{}, nil, err
	}
	metadata, err := qimumcp.CallbackMetadata(ref)
	if err != nil {
		release()
		return sdk.ParamsBase{}, nil, err
	}
	return metadata, release, nil
}

// toolHost grants through the session that admitted the tool, so a replaced
// connection generation is refused rather than handed a stale backend's authority.
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

func (p *Pool) Stream(ctx context.Context, id mcpserver.ID, session *sdk.ClientSession, binding *qimumcp.Binding, command string, input jsonvalue.Value, metadata sdk.ParamsBase) iter.Seq2[qimumcp.StreamValue[jsontext.Value, jsontext.Value], error] {
	return func(yield func(qimumcp.StreamValue[jsontext.Value, jsontext.Value], error) bool) {
		if _, err := p.SessionActor(ctx, id, session); err != nil {
			yield(qimumcp.StreamValue[jsontext.Value, jsontext.Value]{}, err)
			return
		}
		for value, err := range binding.StreamWithMetadata(ctx, p.callbacks, callbackPayloadLimits, command, input, metadata) {
			if !yield(value, err) {
				return
			}
		}
	}
}

type ToolCall func(toolcontract.Invocation) (toolcontract.Result, error)

// ToolHost opens one installed tool call's canonical host grant through Pool.OpenRPC,
// its single owner, so a model Tool reaches the same host capability and frozen
// dependency authority as the backend's plugins.call entry. The caller owns release.
type ToolHost func(context.Context) (sdk.ParamsBase, func() error, error)

// ToolPreparation captures call collaborators for this exact executable. On
// success the caller owns release, even if final authority admission refuses
// dispatch. The prepared call owns work and any dispatch-dependent observation.
// On failure preparation must release any resources it acquired. host opens the
// canonical installed grant when the preparation wants it.
type ToolPreparation func(context.Context, Executable, ToolHost) (ToolCall, func() error, error)
