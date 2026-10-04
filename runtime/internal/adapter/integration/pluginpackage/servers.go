package pluginpackage

import (
	"context"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"strings"

	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
)

func (r *Releases) installationRecord(ctx context.Context, installation *plugin.Installation) (plugin.Record, error) {
	record := installation.Snapshot()
	release, err := r.catalog.Get(ctx, record.Selected.Digest)
	if err != nil {
		return plugin.Record{}, err
	}
	record.Selected = release
	return record, nil
}

// Catalog and dispatch projections never repair the filesystem as a read effect.
// Data preparation belongs to installation changes and connection activation.
func (r *Releases) Prepare(ctx context.Context, installation *plugin.Installation) ([]plugin.Diagnostic, error) {
	if !installation.Active() {
		return nil, nil
	}
	record, err := r.installationRecord(ctx, installation)
	if err != nil {
		return nil, err
	}
	root, _, err := r.verifiedRoot(ctx, record.Selected.Digest)
	if err != nil {
		return nil, err
	}
	if err := root.Close(); err != nil {
		return nil, err
	}
	var diagnostics []plugin.Diagnostic
	for _, server := range record.Selected.Servers {
		if err := ctx.Err(); err != nil {
			return diagnostics, err
		}
		if !installation.ServerEnabled(server.Name) || server.Type != plugin.Stdio {
			continue
		}
		if err := r.prepareBackend(record.ID, server); err != nil {
			diagnostics = append(diagnostics, plugin.Diagnostic{Component: "mcp:" + server.Name, Code: "preparation_failed"})
		}
	}
	return diagnostics, nil
}

func (r *Releases) Connection(ctx context.Context, installation *plugin.Installation, local string) (mcpserver.Server, error) {
	if !installation.ServerEnabled(local) {
		return mcpserver.Server{}, plugin.ErrUnapproved
	}
	record, err := r.installationRecord(ctx, installation)
	if err != nil {
		return mcpserver.Server{}, err
	}
	root, _, err := r.inspectRoot(ctx, record.Selected.Digest)
	if err != nil {
		r.mu.Lock()
		delete(r.verified, record.Selected.Digest)
		r.mu.Unlock()
		return mcpserver.Server{}, errors.Join(plugin.ErrUnavailable, err)
	}
	if err := root.Close(); err != nil {
		return mcpserver.Server{}, err
	}
	for _, server := range record.Selected.Servers {
		if server.Name != local {
			continue
		}
		if server.Type == plugin.Stdio {
			if err := r.prepareBackend(record.ID, server); err != nil {
				return mcpserver.Server{}, errors.Join(plugin.ErrUnavailable, err)
			}
		}
		return realizeServer(installation, record, server, root.Name(), r.dataRoot(record.ID))
	}
	return mcpserver.Server{}, plugin.ErrNotFound
}
func (r *Releases) prepareBackend(id string, server plugin.Server) (err error) {
	host, err := os.OpenRoot(filepath.Dir(r.directory))
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, host.Close()) }()
	if err := host.Mkdir("data", 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	info, err := host.Lstat("data")
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("pluginpackage: data namespace is not a directory")
	}
	namespace, err := host.OpenRoot("data")
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, namespace.Close()) }()
	if err := namespace.Mkdir(id, 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	data := r.dataRoot(id)
	if err := unchangedDataRoot(data); err != nil {
		return err
	}
	if !strings.HasPrefix(server.CWD, "${PLUGIN_DATA}/") {
		return nil
	}
	root, err := namespace.OpenRoot(id)
	if err != nil {
		return err
	}
	err = root.MkdirAll(strings.TrimPrefix(server.CWD, "${PLUGIN_DATA}/"), 0700)
	return errors.Join(err, root.Close())
}

// Descriptors projects admitted declarations without granting execution authority.
// Catalog membership survives an unavailable release or backend directory.
func (r *Releases) Descriptors(ctx context.Context, installation *plugin.Installation) ([]mcpserver.Server, error) {
	record, err := r.installationRecord(ctx, installation)
	if err != nil {
		return nil, err
	}
	dir, err := r.Root(record.Selected.Digest)
	if err != nil {
		return nil, err
	}
	data := r.dataRoot(record.ID)
	servers := make([]mcpserver.Server, 0, len(record.Selected.Servers))
	for _, declared := range record.Selected.Servers {
		server, err := descriptorServer(installation, record, declared, dir, data)
		if err != nil {
			return nil, err
		}
		servers = append(servers, server)
	}
	return servers, nil
}

func (r *Releases) Servers(ctx context.Context, installation *plugin.Installation) (plugins.Backends, error) {
	record, err := r.installationRecord(ctx, installation)
	if err != nil {
		return plugins.Backends{}, err
	}
	root, _, err := r.verifiedRoot(ctx, record.Selected.Digest)
	if err != nil {
		if ctx.Err() != nil {
			return plugins.Backends{}, ctx.Err()
		}
		return plugins.Backends{}, errors.Join(plugin.ErrUnavailable, err)
	}
	if err := root.Close(); err != nil {
		return plugins.Backends{}, err
	}
	dir, err := r.Root(record.Selected.Digest)
	if err != nil {
		return plugins.Backends{}, err
	}
	data := r.dataRoot(record.ID)
	result := plugins.Backends{}
	for _, declared := range record.Selected.Servers {
		if err := ctx.Err(); err != nil {
			return plugins.Backends{}, err
		}
		server, err := realizeServer(installation, record, declared, dir, data)
		if err != nil {
			result.Availability = append(result.Availability, plugin.Diagnostic{Component: "mcp:" + declared.Name, Code: "unavailable_backend"})
			continue
		}
		result.Servers = append(result.Servers, server)
	}
	return result, nil
}

func (r *Releases) dataRoot(id string) string {
	return filepath.Join(filepath.Dir(r.directory), "data", id)
}

func unchangedDataRoot(data string) error {
	resolved, err := filepath.EvalSymlinks(data)
	if err != nil {
		return err
	}
	if resolved != data {
		return errors.New("pluginpackage: data authority changed")
	}
	root, err := os.OpenRoot(data)
	if err != nil {
		return err
	}
	return root.Close()
}

func descriptorServer(installation *plugin.Installation, record plugin.Record, declared plugin.Server, dir, data string) (mcpserver.Server, error) {
	name, err := mcpserver.InstallationServer(record.ID, declared.Name)
	if err != nil {
		return mcpserver.Server{}, err
	}
	server := mcpserver.Server{Name: name, Enabled: installation.ServerEnabled(declared.Name), ReleaseAuthority: installation.ServerAuthority(declared.Name), Description: record.Selected.Description}
	switch declared.Type {
	case plugin.Stdio:
		server.Transport = mcpserver.TransportStdio
		server.Command = declared.Command
		if strings.HasPrefix(server.Command, "./") {
			server.Command = filepath.Join(dir, server.Command[2:])
		}
		for _, arg := range declared.Args {
			server.Args = append(server.Args, expand(arg, dir, data))
		}
		server.Env = map[string]string{"PLUGIN_ROOT": dir, "PLUGIN_DATA": data}
		for key, value := range declared.Env {
			server.Env[key] = expand(value, dir, data)
		}
		server.Dir = dir
		if declared.CWD != "" {
			server.Dir = expand(declared.CWD, dir, data)
			if strings.HasPrefix(server.Dir, "./") {
				server.Dir = filepath.Join(dir, server.Dir[2:])
			}
		}
	case plugin.StreamableHTTP:
		server.Transport = mcpserver.TransportStreamableHTTP
		server.URL = declared.URL
		server.Headers = maps.Clone(declared.Headers)
	default:
		return mcpserver.Server{}, plugin.ErrInvalid
	}
	for _, input := range record.Selected.Inputs {
		value, configured := record.Values[input.ID]
		if input.Server != declared.Name || !configured {
			continue
		}
		switch input.Target {
		case plugin.Environment:
			server.Env[input.Key] = value
		case plugin.Header:
			if server.Headers == nil {
				server.Headers = map[string]string{}
			}
			server.Headers[input.Key] = value
		case plugin.Authorization:
			server.Authorization = value
		}
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
	return server, server.Validate()
}

func realizeServer(installation *plugin.Installation, record plugin.Record, declared plugin.Server, dir, data string) (mcpserver.Server, error) {
	server, err := descriptorServer(installation, record, declared, dir, data)
	if err != nil {
		return mcpserver.Server{}, err
	}
	if !server.Enabled || server.Transport != mcpserver.TransportStdio {
		return server, nil
	}
	if err := unchangedDataRoot(data); err != nil {
		return mcpserver.Server{}, err
	}
	boundary := dir
	if strings.HasPrefix(declared.CWD, "${PLUGIN_DATA}") {
		boundary = data
	}
	relative, err := filepath.Rel(boundary, server.Dir)
	if err != nil || !filepath.IsLocal(relative) {
		return mcpserver.Server{}, errors.Join(plugin.ErrInvalid, err)
	}
	authority, err := os.OpenRoot(boundary)
	if err != nil {
		return mcpserver.Server{}, err
	}
	info, statErr := authority.Stat(relative)
	closeErr := authority.Close()
	if statErr != nil || !info.IsDir() || closeErr != nil {
		return mcpserver.Server{}, errors.Join(plugin.ErrUnavailable, statErr, closeErr)
	}
	return server, nil
}
