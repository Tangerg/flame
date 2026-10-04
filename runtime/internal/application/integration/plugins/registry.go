// Package plugins owns installation admission and derives package contributions.
// MCP connection lifetime and tool policy remain with their established owners.
package plugins

import (
	"context"
	"errors"

	mcpapp "github.com/Tangerg/flame/runtime/internal/application/integration/mcp"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
)

type Store interface {
	List(context.Context) ([]*plugin.Installation, error)
	Get(context.Context, string) (*plugin.Installation, error)
	Save(context.Context, *plugin.Installation) error
	Remove(context.Context, string) error
}
type PackageSources interface {
	Descriptors(context.Context, *plugin.Installation) ([]mcpserver.Server, error)
	Servers(context.Context, *plugin.Installation) (Backends, error)
	Connection(context.Context, *plugin.Installation, string) (mcpserver.Server, error)
}

type userRegistry interface {
	List(context.Context) ([]mcpserver.Server, error)
	Get(context.Context, mcpserver.ServerName) (mcpserver.Server, bool, error)
	Save(context.Context, mcpserver.Server) error
	Remove(context.Context, mcpserver.ServerName) error
	ListExposure(context.Context) ([]tool.Ref, error)
	SetToolExposure(context.Context, tool.Ref, bool) error
}

func (r *Registry) Connection(ctx context.Context, name mcpserver.ServerName) (mcpserver.Server, error) {
	if name.Installation() == "" {
		server, found, err := r.user.Get(ctx, name)
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
	installation, err := r.installations.Get(ctx, name.Installation())
	if errors.Is(err, plugin.ErrNotFound) {
		return mcpserver.Server{}, mcpapp.ErrUnknownServer
	}
	if err != nil {
		return mcpserver.Server{}, err
	}
	return r.packages.Connection(ctx, installation, name.Local())
}

type Backends struct {
	Servers      []mcpserver.Server
	Availability []plugin.Diagnostic
}

// Registry is the only MCP registry projection. Installation descriptors are
// always derived from the installation owner; no editable server rows exist.
type Registry struct {
	user          userRegistry
	installations Store
	packages      PackageSources
}

func NewRegistry(user userRegistry, installations Store, packages PackageSources) (*Registry, error) {
	if user == nil || installations == nil || packages == nil {
		return nil, errors.New("plugins: registry dependencies are required")
	}
	return &Registry{user, installations, packages}, nil
}
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
		descriptors, err := r.packages.Descriptors(ctx, installation)
		if err != nil {
			return nil, err
		}
		backends, err := r.packages.Servers(ctx, installation)
		if context.Cause(ctx) != nil {
			return nil, context.Cause(ctx)
		}
		if err != nil && !errors.Is(err, plugin.ErrUnavailable) {
			return nil, err
		}
		for _, server := range descriptors {
			availability := mcpapp.SourceAvailable
			if errors.Is(err, plugin.ErrUnavailable) {
				availability = mcpapp.SourceUnavailableRelease
			} else {
				for _, diagnostic := range backends.Availability {
					if diagnostic.Component == "mcp:"+server.Name.Local() {
						availability = mcpapp.SourceUnavailableBackend
						break
					}
				}
			}
			result = append(result, mcpapp.Source{Server: server, Availability: availability})
		}
	}
	return result, nil
}

// Definition reads desired configuration and authority independently of executable
// availability. It may describe standing policy, but cannot admit a dispatch.
func (r *Registry) Definition(ctx context.Context, name mcpserver.ServerName) (mcpserver.Server, bool, error) {
	if name.Installation() == "" {
		return r.user.Get(ctx, name)
	}
	installation, err := r.installations.Get(ctx, name.Installation())
	if errors.Is(err, plugin.ErrNotFound) {
		return mcpserver.Server{}, false, nil
	}
	if err != nil {
		return mcpserver.Server{}, false, err
	}
	servers, err := r.packages.Descriptors(ctx, installation)
	if err != nil {
		return mcpserver.Server{}, false, err
	}
	for _, server := range servers {
		if server.Name == name {
			return server, true, nil
		}
	}
	return mcpserver.Server{}, false, nil
}

func (r *Registry) Get(ctx context.Context, name mcpserver.ServerName) (mcpserver.Server, bool, error) {
	if name.Installation() == "" {
		return r.user.Get(ctx, name)
	}
	installation, err := r.installations.Get(ctx, name.Installation())
	if errors.Is(err, plugin.ErrNotFound) {
		return mcpserver.Server{}, false, nil
	}
	if err != nil {
		return mcpserver.Server{}, false, err
	}
	servers, err := r.packages.Servers(ctx, installation)
	if err != nil {
		return mcpserver.Server{}, false, err
	}
	for _, diagnostic := range servers.Availability {
		if diagnostic.Component == "mcp:"+name.Local() {
			return mcpserver.Server{}, false, plugin.ErrUnavailable
		}
	}
	for _, server := range servers.Servers {
		if server.Name == name {
			return server, true, nil
		}
	}
	return mcpserver.Server{}, false, nil
}
func (r *Registry) Save(ctx context.Context, server mcpserver.Server) error {
	if server.Name.Installation() != "" {
		return mcpapp.ErrOwnedByInstallation
	}
	return r.user.Save(ctx, server)
}
func (r *Registry) Remove(ctx context.Context, name mcpserver.ServerName) error {
	if name.Installation() != "" {
		return mcpapp.ErrOwnedByInstallation
	}
	return r.user.Remove(ctx, name)
}
func (r *Registry) ListExposure(ctx context.Context) ([]tool.Ref, error) {
	return r.user.ListExposure(ctx)
}
func (r *Registry) SetToolExposure(ctx context.Context, ref tool.Ref, disabled bool) error {
	return r.user.SetToolExposure(ctx, ref, disabled)
}
