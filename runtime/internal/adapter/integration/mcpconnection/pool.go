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
	"sync"

	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/internal/fingerprint"
	pluginsdk "github.com/Tangerg/flame/runtime/plugin"
	sdk "github.com/Tangerg/go-sdk/mcp"

	mcpapp "github.com/Tangerg/flame/runtime/internal/application/integration/mcp"
	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/dependency"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/infra/integration/mcp"
	"github.com/Tangerg/flame/runtime/protocol"
	"github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

// Pool owns connection lifetime. Every new connection obtains its configuration
// from the source owner; retained configurations describe only existing sessions.
type Pool struct {
	registry  sourceRegistry
	inner     *mcp.Connections
	prepare   ToolPreparation
	publish   invalidation.Publish
	lifetime  context.Context
	callbacks *pluginsdk.Callbacks
	ready     chan struct{}
}

var _ mcpapp.Connections = (*Pool)(nil)

// Startup connection waits belong to the pool, so one optional source cannot block Runtime management.
func Open(
	ctx context.Context,
	lifetime context.Context,
	servers []mcpserver.Server,
	oauthSessions mcp.OAuthSessionStore, registry sourceRegistry,
	callbacks *pluginsdk.Callbacks,
	prepare ToolPreparation,
	publish invalidation.Publish,
) (*Pool, error) {
	if dependency.Missing(registry) {
		return nil, errors.New("mcp connection: source registry is required")
	}
	configs := make([]mcp.ServerConfig, len(servers))
	for index, server := range servers {
		configs[index] = mcp.ServerConfig{Server: server.Clone()}
	}
	pool := &Pool{registry: registry, prepare: prepare, publish: publish, lifetime: lifetime, callbacks: callbacks, ready: make(chan struct{})}
	client := mcp.NewClient()
	if err := pluginsdk.RegisterClient(client); err != nil {
		return nil, err
	}
	inner, err := mcp.Open(ctx, lifetime, configs, oauthSessions, pool.connectionLaunch, callbacks, client)
	if err != nil {
		return nil, err
	}
	pool.inner = inner
	close(pool.ready)
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
	var stdio *mcp.Stdio
	if connection.Stdio != nil {
		executable := connection.Stdio
		stdio = &mcp.Stdio{Command: executable.Command, Args: executable.Args, Env: executable.Env, Dir: executable.Dir}
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
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-p.ready:
			}
			if err := p.waitRequirements(ctx, connection); err != nil {
				return err
			}
			values, err := setupContributions(connection)
			if err != nil {
				return err
			}
			var metadata sdk.ParamsBase
			if len(connection.Requirements.Snapshot()) > 0 {
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
				ref, closeRPC, err := p.callbacks.OpenRPC(operation.WithActor(activationCtx, actor.WithBindings(connection.Requirements)), session, pluginsdk.CallbackLimits{MaxRequestBytes: 4 << 20, MaxResponseBytes: 4 << 20})
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
			return pluginsdk.SetUp(ctx, session, values, metadata)
		}
	}
	return mcp.NewLaunch(mcp.ServerConfig{Server: connection.Server}, stdio, retire, mcp.Admission{Run: admit, Bindings: connection.Requirements})
}

func (p *Pool) Refuse(ctx context.Context, name mcpserver.ID, failure mcpserver.ConnectionFailure) error {
	return p.inner.Refuse(ctx, name, failure)
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
			if _, err := p.inner.WaitSession(ctx, server); err != nil {
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
		for _, required := range connection.Requirements.Snapshot() {
			if required.Contribution != contribution.ID {
				continue
			}
			contract, err := capability.Parse(required.Contract.JSON())
			if err != nil {
				return nil, err
			}
			dependency := pluginsdk.SetupDependency{ID: required.ID.String(), Contract: contract.Descriptor(), Optional: required.Optional, Bindings: []protocol.PluginOperationBinding{}}
			for _, binding := range required.Bindings {
				dependency.Bindings = append(dependency.Bindings, protocol.PluginOperationBinding{InstallationID: binding.Provider.InstallationID.String(), Digest: binding.Provider.Digest.String(), Contribution: binding.Contribution.String(), Contract: contract.Descriptor()})
			}
			value.Requires = append(value.Requires, dependency)
		}
		values = append(values, value)
	}
	return values, nil
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

func (p *Pool) BoundServiceSession(ctx context.Context, id mcpserver.ID, authority fingerprint.Digest) (*sdk.ClientSession, error) {
	descriptor, found, err := p.registry.Dispatchable(ctx, id)
	if err != nil {
		return nil, err
	}
	if !found || !descriptor.Enabled || descriptor.AuthorityFingerprint() != authority {
		return nil, mcp.ErrUnavailable
	}
	return p.inner.WaitSession(ctx, descriptor)
}

// SessionActor captures only the registry source realized by this exact live
// session. The Run transaction rechecks its immutable authority before commit.
func (p *Pool) SessionActor(ctx context.Context, id mcpserver.ID, expected *sdk.ClientSession) (operation.Actor, error) {
	descriptor, found, err := p.registry.Dispatchable(ctx, id)
	if err != nil {
		return operation.Actor{}, err
	}
	if !found || !descriptor.Enabled {
		return operation.Actor{}, mcp.ErrUnavailable
	}
	current, err := p.inner.ServiceSession(ctx, descriptor)
	if err != nil || current != expected {
		return operation.Actor{}, errors.Join(mcp.ErrUnavailable, err)
	}
	release, authority, managed := descriptor.Source.Release()
	if !managed {
		return operation.Actor{}, mcp.ErrUnavailable
	}
	installation, _ := id.Origin().Installation()
	actor, err := operation.NewActor(installation.String(), release.String(), authority.String(), descriptor.Name.String())
	if err != nil {
		return operation.Actor{}, err
	}
	bindings, err := p.inner.Requirements(expected)
	if err != nil {
		return operation.Actor{}, err
	}
	return actor.WithBindings(bindings), nil
}

// OpenRPC preserves the invoking call while applying this backend's frozen
// dependency authority. Retirement is owned by the same session ledger.
func (p *Pool) OpenRPC(ctx context.Context, id mcpserver.ID, session *sdk.ClientSession) (sdk.ParamsBase, func(), error) {
	if p.callbacks == nil {
		return sdk.ParamsBase{}, nil, errors.New("mcp: Runtime callbacks are unavailable")
	}
	actor, err := p.SessionActor(ctx, id, session)
	if err != nil {
		return sdk.ParamsBase{}, nil, err
	}
	ref, release, err := p.callbacks.OpenRPC(operation.WithActor(ctx, actor), session, pluginsdk.CallbackLimits{MaxRequestBytes: 4 << 20, MaxResponseBytes: 4 << 20})
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

type ToolCall func(toolcontract.Invocation) (chat.ToolOutput, error)

// ToolPreparation captures call collaborators for this exact executable. On
// success the caller owns release, even if final authority admission refuses
// dispatch. The prepared call owns work and any dispatch-dependent observation.
// On failure preparation must release any resources it acquired.
type ToolPreparation func(context.Context, mcp.Executable) (ToolCall, func(), error)
