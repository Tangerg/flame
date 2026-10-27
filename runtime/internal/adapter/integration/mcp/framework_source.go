package mcp

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"sync"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/qimu"
	"github.com/Tangerg/flame/qimu/jsonvalue"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/qimu/portable"
	"github.com/Tangerg/flame/qimu/process"
	mcpapp "github.com/Tangerg/flame/runtime/internal/application/integration/mcp"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/samber/lo"
)

type installedPackages interface {
	Connection(context.Context, *plugin.Installation, plugin.Release, mcpserver.ServerName) (mcpapp.Launch, bool, error)
	Server(context.Context, *plugin.Installation, plugin.Release, mcpserver.ServerName, plugins.Reach) (mcpserver.Server, bool, error)
}

// ProcessSource owns Flame's process configuration and callback authority. Qimu
// owns the process, exact imports and resource lifetime. These projections never
// resolve a role or select a replacement provider.
type ProcessSource struct {
	installations plugins.Store
	packages      installedPackages
	callbacks     *qimumcp.Callbacks
	commands      invocationRestorer
	mu            sync.Mutex
	projections   map[string]processProjection
}

type processProjection struct {
	actor       operation.Actor
	fingerprint identity.Digest
	bindings    *plugin.RequiredBindings
}

func NewProcessSource(installations plugins.Store, packages installedPackages, callbacks *qimumcp.Callbacks, commands invocationRestorer) (*ProcessSource, error) {
	if lo.IsNil(installations) || lo.IsNil(packages) || callbacks == nil || lo.IsNil(commands) {
		return nil, errors.New("mcp: installed process source requires original stores, packages, callbacks and invocation owner")
	}
	return &ProcessSource{installations: installations, packages: packages, callbacks: callbacks, commands: commands, projections: map[string]processProjection{}}, nil
}

func (s *ProcessSource) snapshot(ctx context.Context, binding qimu.UnitBinding) (plugin.Snapshot, mcpserver.ServerName, error) {
	id, err := identity.ParseInstallation(binding.Source.InstallationID.String())
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
		return plugin.Snapshot{}, server, qimu.ErrStaleBinding
	}
	configuration, err := plugins.EncodeFrameworkConfiguration(snapshot.Installation.Snapshot())
	if err != nil || qimu.Hash(configuration) != binding.Configuration {
		return plugin.Snapshot{}, server, errors.Join(qimu.ErrStaleBinding, err)
	}
	return snapshot, server, nil
}

func (s *ProcessSource) Program(ctx context.Context, binding qimu.UnitBinding) (program process.Program, err error) {
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
	program.ExecutableDigest = qimu.Digest(launch.Stdio.ExpectedExecutable.String())
	program.Args = slices.Clone(launch.Stdio.Args)
	program.Env = processEnvironment(launch.Stdio.Env)
	program.Directory = launch.Stdio.Dir
	program.CredentialRevision = binding.Configuration.String()
	null, err := jsonvalue.Encode[any](nil)
	if err != nil {
		return program, err
	}
	program.Exports = map[string]string{}
	program.Metadata = map[string]jsonvalue.Value{}
	for _, contribution := range snapshot.Selected.Declaration().Contributions {
		if contribution.Server == server {
			program.Exports[contribution.ID.String()] = contribution.Export
			program.Metadata[contribution.ID.String()] = null
		}
	}
	return program, nil
}

func (s *ProcessSource) Configure(ctx context.Context, setup *qimu.Setup, session *sdk.ClientSession) ([]qimumcp.SetupContribution, sdk.ParamsBase, error) {
	snapshot, serverName, err := s.snapshot(ctx, setup.Binding())
	if err != nil {
		return nil, sdk.ParamsBase{}, err
	}
	server, found, err := s.packages.Server(ctx, snapshot.Installation, snapshot.Selected, serverName, plugins.Declared)
	if err != nil || !found {
		return nil, sdk.ParamsBase{}, errors.Join(plugin.ErrUnavailable, err)
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
	s.projections[setup.InstanceID()] = processProjection{actor: actor, fingerprint: server.AuthorityFingerprint(), bindings: bindings}
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
	var reference qimumcp.CallbackRef
	var closeRPC func()
	if background {
		reference, closeRPC, err = s.callbacks.OpenRPCWithInvocation(calling, session, callbackPayloadLimits, func(ctx context.Context, invocation portable.Invocation) (context.Context, error) {
			ctx, err := s.commands.RestoreInvocation(ctx, invocation)
			if err != nil {
				return nil, err
			}
			return qimumcp.WithInvocation(ctx, invocation)
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
	metadata, err := qimumcp.CallbackMetadata(reference)
	return contributions, metadata, err
}
