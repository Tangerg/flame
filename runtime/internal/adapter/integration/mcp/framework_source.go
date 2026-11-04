package mcp

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"sync"

	framemcp "github.com/Tangerg/frame/process/mcp"
	framevalue "github.com/Tangerg/frame/value"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	mcpapp "github.com/Tangerg/flame/runtime/internal/application/integration/mcp"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/frame"
	sdk "github.com/Tangerg/go-sdk/mcp"

	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"

	"github.com/Tangerg/frame/process"
	"github.com/samber/lo"
)

type installedPackages interface {
	Connection(context.Context, plugin.Installation, plugin.Release, mcpserver.ServerName) (mcpapp.Launch, bool, error)
	Server(context.Context, plugin.Installation, plugin.Release, mcpserver.ServerName, plugins.Reach) (mcpserver.Server, bool, error)
}

// ProcessSource owns Flame's process configuration and callback authority. Frame
// owns the process, exact imports and resource lifetime. These projections never
// resolve a role or select a replacement provider.
type ProcessSource struct {
	installations plugins.Store
	packages      installedPackages
	callbacks     *pluginmcp.Callbacks
	commands      invocationRestorer
	mu            sync.Mutex
	projections   map[string]processProjection
}

type processProjection struct {
	source      frame.UnitRef
	session     *sdk.ClientSession
	actor       operation.Actor
	fingerprint framevalue.Digest
	bindings    *plugin.RequiredBindings
}

func NewProcessSource(installations plugins.Store, packages installedPackages, callbacks *pluginmcp.Callbacks, commands invocationRestorer) (*ProcessSource, error) {
	if lo.IsNil(installations) || lo.IsNil(packages) || callbacks == nil || lo.IsNil(commands) {
		return nil, errors.New("mcp: installed process source requires original stores, packages, callbacks and invocation owner")
	}
	return &ProcessSource{installations: installations, packages: packages, callbacks: callbacks, commands: commands, projections: map[string]processProjection{}}, nil
}

func (s *ProcessSource) snapshot(ctx context.Context, binding frame.UnitBinding) (plugin.Snapshot, mcpserver.ServerName, error) {
	id, err := identity.ParseHostInstallation(binding.Source.InstallationID)
	if err != nil {
		return plugin.Snapshot{}, mcpserver.ServerName{}, err
	}
	server, err := plugins.BackendServer(binding.Source.UnitID)
	if err != nil {
		return plugin.Snapshot{}, server, err
	}
	snapshot, found, err := s.installations.Get(ctx, id)
	if err != nil {
		return plugin.Snapshot{}, server, err
	}
	if !found || !snapshot.Installation.ServerEnabled(server) || snapshot.Selected.Digest().String() != binding.Source.Release.String() {
		return plugin.Snapshot{}, server, frame.ErrStaleBinding
	}
	configuration, err := plugins.EncodeFrameworkConfiguration(snapshot.Installation.Snapshot())
	if err != nil || framevalue.Hash(configuration) != binding.Configuration {
		return plugin.Snapshot{}, server, errors.Join(frame.ErrStaleBinding, err)
	}
	return snapshot, server, nil
}

func (s *ProcessSource) Program(ctx context.Context, binding frame.UnitBinding) (program process.Program, err error) {
	snapshot, server, err := s.snapshot(ctx, binding)
	if err != nil {
		return program, err
	}
	launch, found, err := s.packages.Connection(ctx, snapshot.Installation, snapshot.Selected, server)
	if launch.Retire != nil {
		program.ReleaseContent = launch.Retire.Close
	}
	if err != nil {
		return program, err
	}
	if !found || launch.Stdio == nil {
		return program, errors.New("mcp: installed backend requires the mcp-process profile")
	}
	if !filepath.IsAbs(launch.Stdio.Command) || launch.Stdio.ExpectedExecutable.Validate() != nil {
		return program, errors.New("mcp: installed executable requires an immutable absolute artifact")
	}
	program.Command = launch.Stdio.Command
	program.ExecutableDigest = launch.Stdio.ExpectedExecutable
	program.Args = slices.Clone(launch.Stdio.Args)
	program.Env = processEnvironment(launch.Stdio.Env)
	program.Directory = launch.Stdio.Dir
	program.CredentialRevision = binding.Configuration.String()
	program.Metadata = process.ExportMetadata{}
	for _, contribution := range snapshot.Selected.Declaration().Contributions {
		if contribution.Server == server {
			contract, err := plugincontract.ParseCapability(contribution.Contract.Bytes())
			if err != nil {
				return program, err
			}
			metadata, err := framevalue.EncodeJSON(processExport{Name: contribution.Export, Contract: contract.Descriptor()})
			if err != nil {
				return program, err
			}
			program.Metadata[contribution.ID.String()] = metadata
		}
	}
	return program, nil
}

func (s *ProcessSource) Configure(ctx context.Context, setup *frame.Setup, session *sdk.ClientSession) ([]pluginmcp.SetupContribution, sdk.ParamsBase, error) {
	snapshot, serverName, err := s.snapshot(ctx, setup.Binding())
	if err != nil {
		return nil, sdk.ParamsBase{}, err
	}
	server, found, err := s.packages.Server(ctx, snapshot.Installation, snapshot.Selected, serverName, plugins.Declared)
	if err != nil {
		return nil, sdk.ParamsBase{}, err
	}
	if !found {
		return nil, sdk.ParamsBase{}, plugin.ErrUnavailable
	}
	release, authority, managed := server.Source.Release()
	if !managed {
		return nil, sdk.ParamsBase{}, plugin.ErrInvalid
	}
	actor, err := operation.NewActor(snapshot.Installation.ID().String(), release.String(), authority.String(), serverName.String())
	if err != nil {
		return nil, sdk.ParamsBase{}, err
	}
	bindings, err := plugins.ProjectRequiredBindings(ctx, s.installations, snapshot, serverName, setup.Snapshot())
	if err != nil {
		return nil, sdk.ParamsBase{}, err
	}
	launch := mcpapp.Launch{Server: server, Requirements: bindings}
	background := false
	for _, contribution := range snapshot.Selected.Declaration().Contributions {
		if contribution.Server == serverName {
			launch.Contributions = append(launch.Contributions, contribution)
			background = background || contribution.Background
		}
	}
	contributions, err := setupContributions(launch)
	if err != nil {
		return nil, sdk.ParamsBase{}, err
	}
	actor = actor.WithBindings(bindings)
	s.mu.Lock()
	s.projections[setup.InstanceID()] = processProjection{source: setup.Binding().Source, session: session, actor: actor, fingerprint: server.AuthorityFingerprint(), bindings: bindings}
	s.mu.Unlock()
	releaseAuthority := func() error {
		s.mu.Lock()
		delete(s.projections, setup.InstanceID())
		s.mu.Unlock()
		return nil
	}
	if _, err := setup.Cleanup("Flame process authority", releaseAuthority); err != nil {
		return nil, sdk.ParamsBase{}, errors.Join(err, releaseAuthority())
	}
	if err := s.callbacks.AdmitSession(session); err != nil {
		return nil, sdk.ParamsBase{}, err
	}
	if _, err := setup.Cleanup("Flame callback session", func() error { s.callbacks.RevokeSession(session); return nil }); err != nil {
		s.callbacks.RevokeSession(session)
		return nil, sdk.ParamsBase{}, err
	}
	if !background && len(bindings.Snapshot()) == 0 {
		return contributions, sdk.ParamsBase{}, nil
	}
	if err := s.callbacks.WaitRPC(ctx); err != nil {
		return nil, sdk.ParamsBase{}, err
	}
	calling := operation.WithActor(setup.Lifetime().Context(), actor)
	var reference pluginmcp.CallbackRef
	var closeRPC func()
	if background {
		reference, closeRPC, err = s.callbacks.OpenRPCWithInvocation(calling, session, callbackPayloadLimits, func(ctx context.Context, invocation framemcp.Invocation) (context.Context, error) {
			ctx, err := s.commands.RestoreInvocation(ctx, invocation)
			if err != nil {
				return nil, err
			}
			return pluginmcp.WithInvocation(ctx, invocation)
		})
	} else {
		reference, closeRPC, err = s.callbacks.OpenRPC(calling, session, callbackPayloadLimits)
	}
	if err != nil {
		return nil, sdk.ParamsBase{}, err
	}
	if _, err := setup.Cleanup("Flame process callback grant", func() error { closeRPC(); return nil }); err != nil {
		closeRPC()
		return nil, sdk.ParamsBase{}, err
	}
	metadata, err := pluginmcp.CallbackMetadata(reference)
	return contributions, metadata, err
}
