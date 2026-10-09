package pluginpackage

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	mcpapp "github.com/Tangerg/flame/runtime/internal/application/integration/mcp"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
)

// Catalog and dispatch projections never repair the filesystem as a read effect.
// Data preparation belongs to installation changes and connection activation.
// Its outcome is not retained here: a backend that could not be prepared stays
// unrealizable, and [Releases.Realize] observes that from the filesystem.
func (r *Releases) Prepare(ctx context.Context, installation *plugin.Installation, release plugin.Release) error {
	if !installation.Active() {
		return nil
	}
	root, _, err := r.verifiedRoot(ctx, release.Digest())
	if err != nil {
		return fmt.Errorf("pluginpackage: prepare release %s: %w", release.Digest(), err)
	}
	if err := root.Close(); err != nil {
		return fmt.Errorf("pluginpackage: close release %s: %w", release.Digest(), err)
	}
	var failures error
	for _, server := range release.Declaration().Servers {
		if err := ctx.Err(); err != nil {
			return errors.Join(failures, err)
		}
		if !installation.ServerEnabled(server.Name) || server.Transport != mcpserver.TransportStdio {
			continue
		}
		if err := r.prepareBackend(installation.ID(), server); err != nil {
			failures = errors.Join(failures, fmt.Errorf("pluginpackage: prepare server %q backend: %w", server.Name, err))
		}
	}
	return failures
}

// Server realizes one declared server only as far as reach requires. Siblings
// are never inspected: a call or a connection pays for its own target alone.
// Enablement is reported, not enforced; refusing a disabled server belongs to
// the registry's caller, and a disabled server is never prepared or realized.
func (r *Releases) Server(ctx context.Context, installation *plugin.Installation, release plugin.Release, local mcpserver.ServerName, reach plugins.Reach) (mcpserver.Server, bool, error) {
	declaration := release.Declaration()
	index := slices.IndexFunc(declaration.Servers, func(server plugin.Server) bool { return server.Name == local })
	if index < 0 {
		return mcpserver.Server{}, false, nil
	}
	declared := declaration.Servers[index]
	dir, err := r.Root(release.Digest())
	if err != nil {
		return mcpserver.Server{}, false, fmt.Errorf("pluginpackage: release directory: %w", err)
	}
	data := r.dataRoot(installation.ID())
	server, err := descriptorServer(installation, release, declaration, declared, dir, data)
	if err != nil || reach == plugins.Declared || !server.Enabled {
		return server, err == nil, err
	}
	if reach != plugins.Dispatchable {
		return mcpserver.Server{}, false, fmt.Errorf("pluginpackage: unknown server reach %d", reach)
	}
	root, _, err := r.verifiedRoot(ctx, release.Digest())
	if err != nil {
		if cause := context.Cause(ctx); cause != nil {
			return mcpserver.Server{}, false, cause
		}
		return mcpserver.Server{}, false, fmt.Errorf("%w: verify release %s: %w", plugin.ErrUnavailable, release.Digest(), err)
	}
	if err := root.Close(); err != nil {
		return mcpserver.Server{}, false, fmt.Errorf("pluginpackage: close release %s: %w", release.Digest(), err)
	}
	if err := realizeBackend(server, declared, dir, data); err != nil {
		return mcpserver.Server{}, false, fmt.Errorf("%w: server %q backend: %w", plugin.ErrUnavailable, declared.Name, err)
	}
	return server, true, nil
}
func (r *Releases) prepareBackend(id resourceid.InstallationID, server plugin.Server) (err error) {
	host, err := os.OpenRoot(filepath.Dir(r.directory))
	if err != nil {
		return fmt.Errorf("pluginpackage: open plugin host directory: %w", err)
	}
	defer func() { err = errors.Join(err, host.Close()) }()
	if err := host.Mkdir("data", 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return fmt.Errorf("pluginpackage: create data namespace: %w", err)
	}
	info, err := host.Lstat("data")
	if err != nil {
		return fmt.Errorf("pluginpackage: inspect data namespace: %w", err)
	}
	if !info.IsDir() {
		return errors.New("pluginpackage: data namespace is not a directory")
	}
	namespace, err := host.OpenRoot("data")
	if err != nil {
		return fmt.Errorf("pluginpackage: open data namespace: %w", err)
	}
	defer func() { err = errors.Join(err, namespace.Close()) }()
	if err := namespace.Mkdir(id.String(), 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return fmt.Errorf("pluginpackage: create installation data directory: %w", err)
	}
	data := r.dataRoot(id)
	if err := unchangedDataRoot(data); err != nil {
		return err
	}
	workingDirectory, err := server.WorkingDirectory()
	if err != nil {
		return err
	}
	if workingDirectory.Base() != plugin.DataBase || workingDirectory.Path() == "" {
		return nil
	}
	root, err := namespace.OpenRoot(id.String())
	if err != nil {
		return fmt.Errorf("pluginpackage: open installation data directory: %w", err)
	}
	if err := root.MkdirAll(filepath.FromSlash(workingDirectory.Path()), 0700); err != nil {
		return errors.Join(fmt.Errorf("pluginpackage: create server working directory: %w", err), root.Close())
	}
	return root.Close()
}

// Realize observes the selected release once: whether its bytes verify, and for
// each declared server its descriptor and whether its backend can be realized
// now. Each server is realized on its own, so one that cannot be described or
// realized is reported unavailable without withholding its siblings. Catalog
// membership survives an unavailable release or backend directory, and the
// observation never grants execution authority.
func (r *Releases) Realize(ctx context.Context, installation *plugin.Installation, release plugin.Release) (plugins.Realization, error) {
	dir, err := r.Root(release.Digest())
	if err != nil {
		return plugins.Realization{}, fmt.Errorf("pluginpackage: release directory: %w", err)
	}
	root, verifyErr := r.currentRoot(ctx, release.Digest())
	if cause := context.Cause(ctx); cause != nil {
		if root != nil {
			cause = errors.Join(cause, root.Close())
		}
		return plugins.Realization{}, cause
	}
	result := plugins.Realization{Release: plugins.ReleaseUnavailable}
	if verifyErr == nil {
		if err := root.Close(); err != nil {
			return plugins.Realization{}, fmt.Errorf("pluginpackage: close release %s: %w", release.Digest(), err)
		}
		result.Release = plugins.ReleaseAvailable
	}
	data := r.dataRoot(installation.ID())
	declaration := release.Declaration()
	for _, declared := range declaration.Servers {
		if err := ctx.Err(); err != nil {
			return plugins.Realization{}, err
		}
		server, describeErr := descriptorServer(installation, release, declaration, declared, dir, data)
		if describeErr != nil {
			if server, err = declaredServer(installation, release, declaration, declared); err != nil {
				return plugins.Realization{}, err
			}
		}
		availability := mcpapp.SourceUnavailableRelease
		if result.Release == plugins.ReleaseAvailable {
			availability = mcpapp.SourceAvailable
			if describeErr != nil || realizeBackend(server, declared, dir, data) != nil {
				availability = mcpapp.SourceUnavailableBackend
			}
		}
		result.Sources = append(result.Sources, mcpapp.Source{Server: server, Availability: availability})
	}
	return result, nil
}

// declaredServer is the registry identity of a server whose descriptor cannot
// be realized: enough to list it as unavailable, never enough to connect.
func declaredServer(installation *plugin.Installation, release plugin.Release, declaration plugin.Declaration, declared plugin.Server) (mcpserver.Server, error) {
	source, err := installation.ServerSource(release, declared.Name)
	if err != nil {
		return mcpserver.Server{}, err
	}
	return mcpserver.Server{Source: source, Name: declared.Name, Transport: declared.Transport, Enabled: installation.ServerEnabled(declared.Name), Description: declaration.Description}, nil
}

func (r *Releases) dataRoot(id resourceid.InstallationID) string {
	return filepath.Join(filepath.Dir(r.directory), "data", id.String())
}

func unchangedDataRoot(data string) error {
	resolved, err := filepath.EvalSymlinks(data)
	if err != nil {
		return fmt.Errorf("pluginpackage: resolve installation data directory: %w", err)
	}
	if resolved != data {
		return errors.New("pluginpackage: data authority changed")
	}
	root, err := os.OpenRoot(data)
	if err != nil {
		return fmt.Errorf("pluginpackage: open installation data directory: %w", err)
	}
	return root.Close()
}

func descriptorServer(installation *plugin.Installation, release plugin.Release, declaration plugin.Declaration, declared plugin.Server, dir, data string) (mcpserver.Server, error) {
	server, err := declaredServer(installation, release, declaration, declared)
	if err != nil {
		return mcpserver.Server{}, err
	}
	inputs, err := installation.ServerInputs(release, declared.Name)
	if err != nil {
		return mcpserver.Server{}, err
	}
	switch declared.Transport {
	case mcpserver.TransportStdio:
		server.Command = declared.Command
		if packaged, found := declared.PackagedCommand(); found {
			server.Command = filepath.Join(dir, filepath.FromSlash(packaged))
		}
		for _, arg := range declared.Args {
			server.Args = append(server.Args, plugin.ExpandPlaceholders(arg, dir, data))
		}
		server.Env = map[string]string{plugin.RootVariable: dir, plugin.DataVariable: data}
		for key, value := range declared.Env {
			server.Env[key] = plugin.ExpandPlaceholders(value, dir, data)
		}
		workingDirectory, err := declared.WorkingDirectory()
		if err != nil {
			return mcpserver.Server{}, err
		}
		boundary, relative := locate(workingDirectory, dir, data)
		server.Dir = filepath.Join(boundary, relative)
	case mcpserver.TransportStreamableHTTP:
		server.URL = declared.URL
		server.Headers = maps.Clone(declared.Headers)
	}
	if len(inputs.Headers) > 0 && server.Headers == nil {
		server.Headers = map[string]string{}
	}
	maps.Copy(server.Headers, inputs.Headers)
	maps.Copy(server.Env, inputs.Env)
	if inputs.Authorization != "" {
		server.Authorization = inputs.Authorization
	}
	if server.Transport == mcpserver.TransportStdio {
		inheritPath := true
		for key := range server.Env {
			if strings.EqualFold(key, "PATH") {
				inheritPath = false
				break
			}
		}
		if inheritPath {
			server.Env["PATH"] = os.Getenv("PATH")
		}
	}
	if err := server.Validate(); err != nil {
		return mcpserver.Server{}, fmt.Errorf("pluginpackage: realize server %q: %w", declared.Name, err)
	}
	return server, nil
}

// realizeBackend checks that an enabled stdio descriptor's working directory
// exists inside its boundary now; other descriptors need no backend.
func realizeBackend(server mcpserver.Server, declared plugin.Server, dir, data string) error {
	if !server.Enabled || server.Transport != mcpserver.TransportStdio {
		return nil
	}
	if err := unchangedDataRoot(data); err != nil {
		return err
	}
	workingDirectory, err := declared.WorkingDirectory()
	if err != nil {
		return err
	}
	boundary, relative := locate(workingDirectory, dir, data)
	authority, err := os.OpenRoot(boundary)
	if err != nil {
		return fmt.Errorf("%w: open server %q boundary: %w", plugin.ErrUnavailable, declared.Name, err)
	}
	info, statErr := authority.Stat(relative)
	if err := errors.Join(statErr, authority.Close()); err != nil {
		return fmt.Errorf("%w: server %q working directory: %w", plugin.ErrUnavailable, declared.Name, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%w: server %q working directory is not a directory", plugin.ErrUnavailable, declared.Name)
	}
	return nil
}

// locate resolves a working directory to the boundary it is confined to and
// its path beneath that boundary.
func locate(workingDirectory plugin.WorkingDirectory, dir, data string) (boundary, relative string) {
	boundary = dir
	if workingDirectory.Base() == plugin.DataBase {
		boundary = data
	}
	if workingDirectory.Path() == "" {
		return boundary, "."
	}
	return boundary, filepath.FromSlash(workingDirectory.Path())
}
