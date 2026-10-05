// Package plugins owns installation admission and derives package contributions.
// MCP connection lifetime and tool policy remain with their established owners.
package plugins

import (
	"context"
	"errors"
	"fmt"

	mcpapp "github.com/Tangerg/flame/runtime/internal/application/integration/mcp"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
)

type Store interface {
	List(context.Context) ([]*plugin.Installation, error)
	Get(context.Context, resourceid.InstallationID) (*plugin.Installation, error)
	Save(context.Context, *plugin.Installation) error
	Remove(context.Context, resourceid.InstallationID) error
}
type PackageSources interface {
	Realize(context.Context, *plugin.Installation, plugin.Release) (Realization, error)
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
	// Launchable also revalidates the bytes against tampering and prepares the
	// backend, because a new connection is about to execute them.
	Launchable
)

type userServers interface {
	List(context.Context) ([]mcpserver.Server, error)
	Get(context.Context, mcpserver.ServerName) (mcpserver.Server, bool, error)
}

// Registry is the only MCP registry projection. Installation descriptors are
// always derived from the installation owner; no editable server rows exist.
type Registry struct {
	user          userServers
	installations Store
	catalog       Catalog
	packages      PackageSources
}

func NewRegistry(user userServers, installations Store, catalog Catalog, packages PackageSources) (*Registry, error) {
	if user == nil || installations == nil || catalog == nil || packages == nil {
		return nil, errors.New("plugins: registry dependencies are required")
	}
	return &Registry{user, installations, catalog, packages}, nil
}

func (r *Registry) release(ctx context.Context, installation *plugin.Installation) (plugin.Release, error) {
	release, err := r.catalog.Get(ctx, installation.Selected())
	if err != nil {
		return plugin.Release{}, fmt.Errorf("plugins: read release %s: %w", installation.Selected(), err)
	}
	return release, nil
}

// Catalog lists every source of every origin. Each installation's sources come
// from one realization of its selected release, which reports a server it
// cannot realize as unavailable instead of withholding the rest of the list.
func (r *Registry) Catalog(ctx context.Context) ([]mcpapp.Source, error) {
	users, err := r.user.List(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]mcpapp.Source, 0, len(users))
	for _, server := range users {
		result = append(result, mcpapp.Source{Server: server, Availability: mcpapp.SourceAvailable})
	}
	installations, err := r.installations.List(ctx)
	if err != nil {
		return nil, err
	}
	for _, installation := range installations {
		release, err := r.release(ctx, installation)
		if err != nil {
			return nil, err
		}
		realization, err := r.packages.Realize(ctx, installation, release)
		if err != nil {
			return nil, fmt.Errorf("plugins: realize installation %s: %w", installation.ID(), err)
		}
		result = append(result, realization.Sources...)
	}
	return result, nil
}

// resolve is the only origin dispatch for one server. A user server is its
// stored descriptor; an installation server is realized from the selected
// release only as far as reach requires, and only for the requested server.
func (r *Registry) resolve(ctx context.Context, id mcpserver.ID, reach Reach) (mcpserver.Server, bool, error) {
	installationID, installed := id.Origin().Installation()
	if !installed {
		return r.user.Get(ctx, id.Name())
	}
	installation, err := r.installations.Get(ctx, installationID)
	if errors.Is(err, plugin.ErrNotFound) {
		return mcpserver.Server{}, false, nil
	}
	if err != nil {
		return mcpserver.Server{}, false, fmt.Errorf("plugins: read installation %s: %w", installationID, err)
	}
	release, err := r.release(ctx, installation)
	if err != nil {
		return mcpserver.Server{}, false, err
	}
	return r.packages.Server(ctx, installation, release, id.Name(), reach)
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

// Connection realizes the configuration a new connection launches with and
// refuses an unknown or disabled server.
func (r *Registry) Connection(ctx context.Context, id mcpserver.ID) (mcpserver.Server, error) {
	server, found, err := r.resolve(ctx, id, Launchable)
	if err != nil {
		return mcpserver.Server{}, err
	}
	if !found {
		return mcpserver.Server{}, mcpapp.ErrUnknownServer
	}
	if !server.Enabled {
		return mcpserver.Server{}, mcpapp.ErrServerDisabled
	}
	return server, nil
}
