// Package plugins owns installation admission and derives package contributions.
// MCP connection lifetime and tool policy remain with their established owners.
package plugins

import (
	"context"
	"fmt"
	"slices"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/qimu"
	mcpapp "github.com/Tangerg/flame/runtime/internal/application/integration/mcp"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/samber/lo"
)

type Store interface {
	List(context.Context) ([]plugin.Snapshot, error)
	Get(context.Context, identity.InstallationID) (plugin.Snapshot, bool, error)
	Stage(context.Context, identity.InstallationID, identity.Digest) error
}
type packageSources interface {
	Definitions(context.Context, *plugin.Installation, plugin.Release) ([]mcpserver.Server, error)
	Server(context.Context, *plugin.Installation, plugin.Release, mcpserver.ServerName, Reach) (mcpserver.Server, bool, error)
}

// Reach is how far a single-server read realizes an installation's release.
// Each step adds to the one before it; user servers have no release to realize.
type Reach uint8

const (
	// Declared projects desired configuration and standing policy without
	// consulting package bytes, so it can never admit a dispatch.
	Declared Reach = iota + 1
	// Dispatchable requires verified release bytes and a realizable backend for
	// the target server: the authority an admitted tool call rechecks.
	Dispatchable
)

type userServers interface {
	List(context.Context) ([]mcpserver.Server, error)
	Get(context.Context, mcpserver.ServerName) (mcpserver.Server, bool, error)
}

// Registry is the only MCP registry projection. Installation descriptors are
// always derived from the installation owner; no editable server rows exist.
type Registry struct {
	framework     *qimu.Host
	user          userServers
	installations Store
	packages      packageSources
	selections    SelectionStore
	compositions  CompositionStore
}

func NewRegistry(host *qimu.Host, user userServers, installations Store, packages packageSources, selections SelectionStore, compositions CompositionStore) (*Registry, error) {
	for _, required := range []struct {
		name  string
		value any
	}{
		{"framework host", host},
		{"user server store", user}, {"installation store", installations}, {"package sources", packages}, {"selection store", selections},
		{"composition store", compositions},
	} {
		if lo.IsNil(required.value) {
			return nil, fmt.Errorf("plugins: registry %s is required", required.name)
		}
	}
	return &Registry{framework: host, user: user, installations: installations, packages: packages, selections: selections, compositions: compositions}, nil
}

func (r *Registry) Framework() *qimu.Host { return r.framework }

// ComponentPlan prepares one immutable component export in its target Realm.
// The Host revalidates the plan before the browser receives execution authority.
func (r *Registry) ComponentPlan(ctx context.Context, realm qimu.RealmID, id identity.InstallationID, digest identity.Digest, component plugin.ContributionID) (*qimu.BindingPlan, error) {
	snapshot, found, err := r.installations.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, plugin.ErrNotFound
	}
	if _, err := snapshot.Installation.AuthorizeComponent(snapshot.Selected, digest, component); err != nil {
		return nil, err
	}
	contract, err := ComponentContract()
	if err != nil {
		return nil, err
	}
	return r.framework.PlanWith(ctx, realm, nil, qimu.ExportRef{
		InstallationID: qimu.InstallationID(id.String()),
		Release:        qimu.Digest(digest.String()),
		UnitID:         ComponentUnit(component),
		Export:         "component",
		Contract:       contract.Ref(),
	})
}

// Definitions projects desired configuration without observing executable bytes
// or backend availability. Connection admission and plugin inspection own those reads.
func (r *Registry) Definitions(ctx context.Context) ([]mcpserver.Server, error) {
	users, err := r.user.List(ctx)
	if err != nil {
		return nil, err
	}
	result := users
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

// resolve reads one source without acquiring launch resources. A user server is its
// stored descriptor; an installation server is realized from the selected
// release only as far as reach requires, and only for the requested server.
func (r *Registry) resolve(ctx context.Context, id mcpserver.ID, reach Reach) (mcpserver.Server, bool, error) {
	installationID, installed := id.Origin().Installation()
	if !installed {
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

// Definition reads desired configuration and authority independently of executable
// availability. It may describe standing policy, but cannot admit a dispatch.
func (r *Registry) Definition(ctx context.Context, id mcpserver.ID) (mcpserver.Server, bool, error) {
	return r.resolve(ctx, id, Declared)
}

// Dispatchable reads the current executable authority an admitted call must
// still hold.
func (r *Registry) Dispatchable(ctx context.Context, id mcpserver.ID) (mcpserver.Server, bool, error) {
	return r.resolve(ctx, id, Dispatchable)
}

// BackendPlan captures consumer choices once for Qimu admission of the complete
// exact Unit graph, including an entry with no public export.
func (r *Registry) BackendPlan(ctx context.Context, id mcpserver.ID) (*qimu.BindingPlan, error) {
	installationID, installed := id.Origin().Installation()
	if !installed {
		return nil, plugin.ErrInvalid
	}
	observed, exists, err := r.installations.Get(ctx, installationID)
	if err != nil {
		return nil, err
	}
	if !exists || !slices.ContainsFunc(observed.Selected.Declaration().Servers, func(server plugin.Server) bool { return server.Name == id.Name() }) {
		return nil, mcpapp.ErrUnknownServer
	}
	catalog, err := readRequirementCatalog(ctx, r.installations, r.selections, r.compositions)
	if err != nil {
		return nil, err
	}
	root, found := catalog.installations[installationID]
	if !found {
		return nil, mcpapp.ErrUnknownServer
	}
	if !root.Installation.ServerEnabled(id.Name()) {
		return nil, mcpapp.ErrServerDisabled
	}
	return catalog.plan(ctx, r.framework, root, id.Name())
}

// Connection supplies ordinary user-owned MCP configuration. Installed Units
// are materialized only through BackendPlan and Qimu's execution profiles.
func (r *Registry) Connection(ctx context.Context, id mcpserver.ID) (mcpapp.Launch, error) {
	if _, installed := id.Origin().Installation(); installed {
		return mcpapp.Launch{}, fmt.Errorf("%w: installed backend requires an approved Qimu Use", plugin.ErrInvalid)
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
