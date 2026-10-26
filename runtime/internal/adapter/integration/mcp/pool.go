package mcp

import (
	"context"
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"iter"
	"maps"
	"slices"
	"sync"

	"github.com/Tangerg/flame/packages/go-sdk/capability"
	"github.com/Tangerg/flame/packages/go-sdk/fingerprint"
	"github.com/Tangerg/flame/packages/go-sdk/jsoninput"
	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	sdk "github.com/Tangerg/go-sdk/mcp"

	"github.com/Tangerg/flame/packages/go-sdk/dependency"
	mcpapp "github.com/Tangerg/flame/runtime/internal/application/integration/mcp"
	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"

	toolcontract "github.com/Tangerg/scope/core/tool"
)

// callbackPayloadLimits caps a plugin callback's request and response bodies at 4 MiB.
var callbackPayloadLimits = pluginsdk.CallbackLimits{MaxRequestBytes: 4 << 20, MaxResponseBytes: 4 << 20}

// Pool owns connection lifetime. Every new connection obtains its configuration
// from the source owner; retained configurations describe only existing sessions.
type Pool struct {
	commands  *plugins.Commands
	registry  sourceRegistry
	inner     *connections
	prepare   ToolPreparation
	publish   invalidation.Publish
	lifetime  context.Context
	callbacks *pluginsdk.Callbacks
}

var _ mcpapp.Connections = (*Pool)(nil)

// Startup connection waits belong to the pool, so one optional source cannot block Runtime management.
func Open(
	ctx context.Context,
	lifetime context.Context,
	servers []mcpserver.Server,
	oauthSessions oauthSessionStore, registry sourceRegistry,
	callbacks *pluginsdk.Callbacks,
	prepare ToolPreparation,
	publish invalidation.Publish,
	commands *plugins.Commands,
) (*Pool, error) {
	if dependency.Missing(registry) {
		return nil, errors.New("mcp connection: source registry is required")
	}
	configs := make([]ServerConfig, len(servers))
	for index, server := range servers {
		configs[index] = ServerConfig{Server: server.Clone()}
	}
	pool := &Pool{commands: commands, registry: registry, prepare: prepare, publish: publish, lifetime: lifetime, callbacks: callbacks}
	client := newClient()
	if err := pluginsdk.RegisterClient(client); err != nil {
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
	connection, err := p.registry.Connection(ctx, name)
	if err != nil {
		if errors.Is(err, mcpapp.ErrUnknownServer) || errors.Is(err, mcpapp.ErrServerDisabled) {
			err = errors.Join(errSourceWithdrawn, err)
		}
		return nil, err
	}
	var retire func() error
	if connection.Retire != nil {
		retire = connection.Retire.Close
	}
	activationCtx, cancelActivation := context.WithCancel(p.lifetime)
	var releaseRPC func()
	var rpcMu sync.Mutex
	retireSource := retire
	retire = func() error {
		cancelActivation()
		rpcMu.Lock()
		release := releaseRPC
		releaseRPC = nil
		rpcMu.Unlock()
		if release != nil {
			release()
		}
		if retireSource != nil {
			return retireSource()
		}
		return nil
	}
	var stdio *stdioLaunch
	if connection.Stdio != nil {
		executable := connection.Stdio
		stdio = &stdioLaunch{Command: executable.Command, Args: executable.Args, Env: executable.Env, Dir: executable.Dir}
	}
	var admit func(context.Context, *sdk.ClientSession) error
	if len(connection.Exports) > 0 {
		exports := make([]pluginsdk.CapabilityExport, 0, len(connection.Exports))
		for _, name := range slices.Sorted(maps.Keys(connection.Exports)) {
			identity := connection.Exports[name]
			contract, err := capability.Parse(identity.JSON())
			if err != nil {
				if retire != nil {
					err = errors.Join(err, retire())
				}
				return nil, err
			}
			exports = append(exports, pluginsdk.CapabilityExport{Name: name, Contract: contract.Descriptor()})
		}
		admit = func(ctx context.Context, session *sdk.ClientSession) error {
			if err := pluginsdk.VerifyCapabilities(ctx, session, exports); err != nil {
				return err
			}
			if err := p.waitRequirements(ctx, connection); err != nil {
				return err
			}
			values, err := setupContributions(connection)
			if err != nil {
				return err
			}
			var metadata sdk.ParamsBase
			background := slices.ContainsFunc(connection.Contributions, func(value plugin.Contribution) bool { return value.Background })
			if background || len(connection.Requirements.Snapshot()) > 0 {
				if p.callbacks == nil {
					return errors.New("mcp: declared collaborators require Runtime callbacks")
				}
				if err := p.callbacks.WaitRPC(ctx); err != nil {
					return err
				}
				release, authority, _ := connection.Server.Source.Release()
				installation, _ := connection.Server.ID().Origin().Installation()
				actor, err := operation.NewActor(installation.String(), release.String(), authority.String(), connection.Server.Name.String())
				if err != nil {
					return err
				}
				calling := operation.WithActor(activationCtx, actor.WithBindings(connection.Requirements))
				limits := callbackPayloadLimits
				var ref pluginsdk.CallbackRef
				var closeRPC func()
				if background {
					if p.commands == nil {
						return errors.New("mcp: background role requires original invocation routing")
					}
					ref, closeRPC, err = p.callbacks.OpenRPCWithInvocation(calling, session, limits, func(ctx context.Context, invocation capability.Invocation) (context.Context, error) {
						ctx, err := p.commands.RestoreInvocation(ctx, invocation)
						if err != nil {
							return nil, err
						}
						return pluginsdk.WithInvocation(ctx, invocation)
					})
				} else {
					ref, closeRPC, err = p.callbacks.OpenRPC(calling, session, limits)
				}
				if err != nil {
					return err
				}
				rpcMu.Lock()
				if err := activationCtx.Err(); err != nil {
					rpcMu.Unlock()
					closeRPC()
					return err
				}
				releaseRPC = closeRPC
				rpcMu.Unlock()
				metadata, err = pluginsdk.CallbackMetadata(ref)
				if err != nil {
					return err
				}
			}
			if err := pluginsdk.SetUp(ctx, session, values, metadata); err != nil {
				return err
			}
			return pluginsdk.Activate(ctx, session)
		}
	}
	return newLaunch(ServerConfig{Server: connection.Server}, stdio, retire, admission{Run: admit, Bindings: connection.Requirements})
}

func (p *Pool) Refuse(ctx context.Context, name mcpserver.ID, failure mcpserver.ConnectionFailure) error {
	return p.inner.refuse(ctx, name, failure)
}

func (p *Pool) waitRequirements(ctx context.Context, connection mcpapp.Launch) error {
	for _, required := range connection.Requirements.Snapshot() {
		if required.Optional {
			continue
		}
		for _, binding := range required.Bindings {
			origin, err := mcpserver.InstallationOrigin(binding.Provider.InstallationID)
			if err != nil {
				return err
			}
			id, err := mcpserver.NewID(origin, binding.Server)
			if err != nil {
				return err
			}
			if id == connection.Server.ID() {
				continue
			}
			server, found, err := p.registry.Dispatchable(ctx, id)
			if err != nil {
				return err
			}
			digest, _, managed := server.Source.Release()
			if !found || !server.Enabled || !managed || digest != binding.Provider.Digest {
				return fmt.Errorf("mcp: required dependency %s/%s: %w", required.Contribution, required.ID, plugin.ErrUnavailable)
			}
			if _, err := p.inner.waitSession(ctx, server); err != nil {
				return fmt.Errorf("mcp: required dependency %s/%s: %w", required.Contribution, required.ID, err)
			}
		}
	}
	return nil
}

func setupContributions(connection mcpapp.Launch) ([]pluginsdk.SetupContribution, error) {
	values := make([]pluginsdk.SetupContribution, 0, len(connection.Contributions))
	for _, contribution := range connection.Contributions {
		value := pluginsdk.SetupContribution{ID: contribution.ID.String(), Export: contribution.Export, Requires: []pluginsdk.SetupDependency{}}
		if contribution.Background {
			installation, found := connection.Server.ID().Origin().Installation()
			if !found {
				return nil, errors.New("mcp: background role requires an installed entry")
			}
			value.Background = &capability.Target{Scope: capability.ScopeInstallation, Reference: installation.String()}
		}
		for _, required := range connection.Requirements.Snapshot() {
			if required.Contribution != contribution.ID {
				continue
			}
			contract, err := capability.Parse(required.Contract.JSON())
			if err != nil {
				return nil, err
			}
			dependency := pluginsdk.SetupDependency{ID: required.ID.String(), Contract: contract.Descriptor(), Optional: required.Optional, Bindings: []pluginsdk.SetupBinding{}}
			for _, binding := range required.Bindings {
				dependency.Bindings = append(dependency.Bindings, pluginsdk.SetupBinding{InstallationID: binding.Provider.InstallationID.String(), Digest: binding.Provider.Digest.String(), Contribution: binding.Contribution.String(), Contract: contract.Descriptor()})
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
func (p *Pool) SetToolSink(sink func([]toolcontract.Tool, []mcpserver.ID)) {
	p.inner.setToolSink(func(catalog []Executable, pending, changed []mcpserver.ID) {
		sink(p.authorizedTools(catalog), pending)
		if len(changed) > 0 {
			p.publish.Notify(invalidation.ForMCP(changed...))
		}
	})
}

// Shutdown releases every live connection under the caller's shutdown budget.
func (p *Pool) Shutdown(ctx context.Context) error {
	return p.inner.shutdown(ctx)
}

func (p *Pool) ServiceSession(ctx context.Context, id mcpserver.ID) (*sdk.ClientSession, error) {
	descriptor, found, err := p.registry.Dispatchable(ctx, id)
	if err != nil {
		return nil, err
	}
	if !found || !descriptor.Enabled {
		return nil, ErrUnavailable
	}
	return p.inner.serviceSession(ctx, descriptor)
}

func (p *Pool) InstallationActivation(ctx context.Context, id mcpserver.ID) (plugin.Activation, error) {
	session, err := p.ServiceSession(ctx, id)
	var activation plugin.Activation
	if err == nil {
		activation, err = p.inner.activation(session)
	}
	if errors.Is(err, ErrUnavailable) {
		// Availability is known at this capture boundary. Callers still retain
		// uncertainty when dispatch may already have occurred.
		err = errors.Join(plugin.ErrUnavailable, err)
	}
	return activation, err
}

func (p *Pool) BoundServiceSession(ctx context.Context, id mcpserver.ID, authority fingerprint.Digest) (*sdk.ClientSession, error) {
	descriptor, found, err := p.registry.Dispatchable(ctx, id)
	if err != nil {
		return nil, err
	}
	if !found || !descriptor.Enabled || descriptor.AuthorityFingerprint() != authority {
		return nil, ErrUnavailable
	}
	return p.inner.waitSession(ctx, descriptor)
}

// SessionActor captures only the registry source realized by this exact live
// session. The Run transaction rechecks its immutable authority before commit.
func (p *Pool) SessionActor(ctx context.Context, id mcpserver.ID, expected *sdk.ClientSession) (operation.Actor, error) {
	descriptor, found, err := p.registry.Dispatchable(ctx, id)
	if err != nil {
		return operation.Actor{}, err
	}
	if !found || !descriptor.Enabled {
		return operation.Actor{}, ErrUnavailable
	}
	current, err := p.inner.serviceSession(ctx, descriptor)
	if err != nil || current != expected {
		return operation.Actor{}, errors.Join(ErrUnavailable, err)
	}
	release, authority, managed := descriptor.Source.Release()
	if !managed {
		return operation.Actor{}, ErrUnavailable
	}
	installation, _ := id.Origin().Installation()
	actor, err := operation.NewActor(installation.String(), release.String(), authority.String(), descriptor.Name.String())
	if err != nil {
		return operation.Actor{}, err
	}
	bindings, err := p.inner.requirements(expected)
	if err != nil {
		return operation.Actor{}, err
	}
	return actor.WithBindings(bindings), nil
}

// OpenRPC preserves the invoking call while applying this backend's frozen
// dependency authority. Retirement is owned by the same session ledger.
func (p *Pool) OpenRPC(ctx context.Context, id mcpserver.ID, session *sdk.ClientSession, callbacks ...pluginsdk.Callback) (sdk.ParamsBase, func(), error) {
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
	metadata, err := pluginsdk.CallbackMetadata(ref)
	if err != nil {
		release()
		return sdk.ParamsBase{}, nil, err
	}
	return metadata, release, nil
}

// toolHost grants through the session that admitted the tool, so a replaced
// connection generation is refused rather than handed a stale backend's authority.
func (p *Pool) toolHost(executable Executable) ToolHost {
	return func(ctx context.Context) (sdk.ParamsBase, func(), error) {
		session, err := executable.CallbackSession()
		if err != nil {
			return sdk.ParamsBase{}, nil, err
		}
		return p.OpenRPC(ctx, executable.SourceConfig().ID(), session)
	}
}

func (p *Pool) Stream(ctx context.Context, id mcpserver.ID, session *sdk.ClientSession, binding *pluginsdk.Binding, command string, input jsoninput.Input, metadata sdk.ParamsBase) iter.Seq2[pluginsdk.StreamValue[jsontext.Value, jsontext.Value], error] {
	return func(yield func(pluginsdk.StreamValue[jsontext.Value, jsontext.Value], error) bool) {
		if _, err := p.SessionActor(ctx, id, session); err != nil {
			yield(pluginsdk.StreamValue[jsontext.Value, jsontext.Value]{}, err)
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
type ToolHost func(context.Context) (sdk.ParamsBase, func(), error)

// ToolPreparation captures call collaborators for this exact executable. On
// success the caller owns release, even if final authority admission refuses
// dispatch. The prepared call owns work and any dispatch-dependent observation.
// On failure preparation must release any resources it acquired. host opens the
// canonical installed grant when the preparation wants it.
type ToolPreparation func(context.Context, Executable, ToolHost) (ToolCall, func(), error)
