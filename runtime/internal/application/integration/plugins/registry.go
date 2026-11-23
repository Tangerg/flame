package plugins

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/identity"
	mcpapp "github.com/Tangerg/flame/runtime/internal/application/integration/mcp"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"

	framehost "github.com/Tangerg/frame/host"
	framevalue "github.com/Tangerg/frame/value"
	"github.com/samber/lo"
)

type Store interface {
	List(context.Context) ([]plugin.Snapshot, error)
	Get(context.Context, identity.InstallationID) (plugin.Snapshot, bool, error)
	Stage(context.Context, identity.InstallationID, framevalue.Digest) error
}
type packageSources interface {
	Definitions(context.Context, plugin.Installation, plugin.Release) ([]mcpserver.Server, error)
	Server(context.Context, plugin.Installation, plugin.Release, mcpserver.ServerName, Reach) (mcpserver.Server, bool, error)
}

type Reach uint8

const (
	Declared Reach = iota + 1
	Dispatchable
)

type userServers interface {
	List(context.Context) ([]mcpserver.Server, error)
	Get(context.Context, mcpserver.ServerName) (mcpserver.Server, bool, error)
}

type Registry struct {
	framework     framehost.Host
	user          userServers
	installations Store
	packages      packageSources
	selections    SelectionStore
	compositions  CompositionStore
}

func NewRegistry(host framehost.Host, installations Store, packages packageSources, selections SelectionStore, compositions CompositionStore) (*Registry, error) {
	if !host.Valid() {
		return nil, errors.New("plugins: registry framework host is required")
	}
	for _, required := range []struct {
		name  string
		value any
	}{
		{"installation store", installations}, {"package sources", packages}, {"selection store", selections},
		{"composition store", compositions},
	} {
		if lo.IsNil(required.value) {
			return nil, fmt.Errorf("plugins: registry %s is required", required.name)
		}
	}
	return &Registry{framework: host, installations: installations, packages: packages, selections: selections, compositions: compositions}, nil
}

func (r *Registry) WithExternalDirectory(user userServers) *Registry {
	copied := *r
	copied.user = user
	return &copied
}

func (r *Registry) ComponentPlan(ctx context.Context, realm framehost.RealmID, id identity.InstallationID, digest framevalue.Digest, component plugin.ContributionID) (framehost.BindingPlan, error) {
	snapshot, found, err := r.installations.Get(ctx, id)
	if err != nil {
		return framehost.BindingPlan{}, err
	}
	if !found {
		return framehost.BindingPlan{}, plugin.ErrNotFound
	}
	if _, err := snapshot.Installation.AuthorizeComponent(snapshot.Selected, digest, component); err != nil {
		return framehost.BindingPlan{}, err
	}
	contract, err := ComponentContract()
	if err != nil {
		return framehost.BindingPlan{}, err
	}
	return r.framework.PlanWith(ctx, realm, nil, framehost.ExportRef{
		InstallationID: id.Host(),
		Release:        digest,
		UnitID:         ComponentUnit(component),
		Export:         extension.ComponentExport,
		Contract:       contract.Ref(),
	})
}

func (r *Registry) Definitions(ctx context.Context) ([]mcpserver.Server, error) {
	var result []mcpserver.Server
	if r.user != nil {
		users, err := r.user.List(ctx)
		if err != nil {
			return nil, err
		}
		result = users
	}
	installations, err := r.installations.List(ctx)
	if err != nil {
		return nil, err
	}
	for _, snapshot := range installations {
		installation := snapshot.Installation
		servers, err := r.packages.Definitions(ctx, installation, snapshot.Selected)
		if err != nil {
			return nil, fmt.Errorf("plugins: describe installation %s: %w", installation.ID(), err)
		}
		result = append(result, servers...)
	}
	return result, nil
}

func (r *Registry) resolve(ctx context.Context, id mcpserver.ID, reach Reach) (mcpserver.Server, bool, error) {
	installationID, installed := id.Origin().Installation()
	if !installed {
		if r.user == nil {
			return mcpserver.Server{}, false, nil
		}
		return r.user.Get(ctx, id.Name())
	}
	snapshot, found, err := r.installations.Get(ctx, installationID)
	if err != nil {
		return mcpserver.Server{}, false, fmt.Errorf("plugins: read installation %s: %w", installationID, err)
	}
	if !found {
		return mcpserver.Server{}, false, nil
	}
	return r.packages.Server(ctx, snapshot.Installation, snapshot.Selected, id.Name(), reach)
}

func (r *Registry) Definition(ctx context.Context, id mcpserver.ID) (mcpserver.Server, bool, error) {
	return r.resolve(ctx, id, Declared)
}

func (r *Registry) Dispatchable(ctx context.Context, id mcpserver.ID) (mcpserver.Server, bool, error) {
	return r.resolve(ctx, id, Dispatchable)
}

func (r *Registry) BackendPlan(ctx context.Context, id mcpserver.ID) (framehost.BindingPlan, error) {
	installationID, installed := id.Origin().Installation()
	if !installed {
		return framehost.BindingPlan{}, plugin.ErrInvalid
	}
	observed, exists, err := r.installations.Get(ctx, installationID)
	if err != nil {
		return framehost.BindingPlan{}, err
	}
	if !exists || !slices.ContainsFunc(observed.Selected.Declaration().Servers, func(server plugin.Server) bool { return server.Name == id.Name() }) {
		return framehost.BindingPlan{}, mcpapp.ErrUnknownServer
	}
	catalog, err := readRequirementCatalog(ctx, r.installations, r.selections, r.compositions)
	if err != nil {
		return framehost.BindingPlan{}, err
	}
	root, found := catalog.installations[installationID]
	if !found {
		return framehost.BindingPlan{}, mcpapp.ErrUnknownServer
	}
	if !root.Installation.ServerEnabled(id.Name()) {
		return framehost.BindingPlan{}, mcpapp.ErrServerDisabled
	}
	return catalog.plan(ctx, r.framework, root, id.Name())
}

func (r *Registry) Connection(ctx context.Context, id mcpserver.ID) (mcpapp.Launch, error) {
	if _, installed := id.Origin().Installation(); installed {
		return mcpapp.Launch{}, fmt.Errorf("%w: installed backend requires an approved Frame Use", plugin.ErrInvalid)
	}
	server, found, err := r.user.Get(ctx, id.Name())
	if err != nil {
		return mcpapp.Launch{}, err
	}
	if !found {
		return mcpapp.Launch{}, mcpapp.ErrUnknownServer
	}
	if !server.Enabled {
		return mcpapp.Launch{}, mcpapp.ErrServerDisabled
	}
	return mcpapp.Launch{Server: server}, nil
}
