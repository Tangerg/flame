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

	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
	"github.com/Tangerg/flame/packages/go-sdk/identity"
	pluginprocess "github.com/Tangerg/flame/packages/go-sdk/plugin/process"
	localruntime "github.com/Tangerg/flame/packages/local-runtime"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
)

func (r *Releases) Prepare(ctx context.Context, installation plugin.Installation, release plugin.Release) error {
	if !installation.Enabled() {
		return nil
	}
	lease, err := r.content.Acquire(ctx, release.Digest())
	if err != nil {
		return fmt.Errorf("pluginpackage: prepare release %s: %w", release.Digest(), err)
	}
	if err := lease.Close(); err != nil {
		return fmt.Errorf("pluginpackage: close release %s: %w", release.Digest(), err)
	}
	var failures error
	for _, server := range release.Declaration().Servers {
		if err := errors.Join(ctx.Err(), context.Cause(ctx)); err != nil {
			return errors.Join(failures, err)
		}
		if !installation.ServerEnabled(server.Name) || server.Transport() != mcpserver.TransportStdio {
			continue
		}
		if err := r.prepareBackend(installation.ID(), server); err != nil {
			failures = errors.Join(failures, fmt.Errorf("pluginpackage: prepare server %q backend: %w", server.Name, err))
		}
	}
	return failures
}

func (r *Releases) Server(ctx context.Context, installation plugin.Installation, release plugin.Release, local mcpserver.ServerName, reach plugins.Reach) (_ mcpserver.Server, _ bool, err error) {
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
	lease, err := r.content.Acquire(ctx, release.Digest())
	if err != nil {
		if cause := errors.Join(ctx.Err(), context.Cause(ctx)); cause != nil {
			return mcpserver.Server{}, false, errors.Join(err, cause)
		}
		return mcpserver.Server{}, false, fmt.Errorf("%w: verify release %s: %w", plugin.ErrUnavailable, release.Digest(), err)
	}
	defer func() { err = errors.Join(err, lease.Close()) }()
	if _, err := selectedVariant(lease.Snapshot().Manifest, declared.Name); err != nil {
		return mcpserver.Server{}, false, err
	}
	if err := realizeBackend(ctx, server, declared, dir, data); err != nil {
		return mcpserver.Server{}, false, fmt.Errorf("%w: server %q backend: %w", plugin.ErrUnavailable, declared.Name, err)
	}
	return server, true, nil
}

func (r *Releases) prepareBackend(id identity.InstallationID, server plugin.Server) (err error) {
	host, err := os.OpenRoot(filepath.Dir(r.dataDirectory))
	if err != nil {
		return fmt.Errorf("pluginpackage: open plugin host directory: %w", err)
	}
	defer func() { err = errors.Join(err, host.Close()) }()
	namespaceName := filepath.Base(r.dataDirectory)
	if err := host.Mkdir(namespaceName, filesystem.PrivateDirectoryMode); err != nil && !errors.Is(err, os.ErrExist) {
		return fmt.Errorf("pluginpackage: create data namespace: %w", err)
	}
	info, err := host.Lstat(namespaceName)
	if err != nil {
		return fmt.Errorf("pluginpackage: inspect data namespace: %w", err)
	}
	if !info.IsDir() {
		return errors.New("pluginpackage: data namespace is not a directory")
	}
	namespace, err := host.OpenRoot(namespaceName)
	if err != nil {
		return fmt.Errorf("pluginpackage: open data namespace: %w", err)
	}
	defer func() { err = errors.Join(err, namespace.Close()) }()
	if err := namespace.Mkdir(id.String(), filesystem.PrivateDirectoryMode); err != nil && !errors.Is(err, os.ErrExist) {
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
	if err := root.MkdirAll(filepath.FromSlash(workingDirectory.Path()), filesystem.PrivateDirectoryMode); err != nil {
		return errors.Join(fmt.Errorf("pluginpackage: create server working directory: %w", err), root.Close())
	}
	return root.Close()
}

func (r *Releases) Definitions(ctx context.Context, installation plugin.Installation, release plugin.Release) ([]mcpserver.Server, error) {
	dir, err := r.Root(release.Digest())
	if err != nil {
		return nil, fmt.Errorf("pluginpackage: release directory: %w", err)
	}
	data := r.dataRoot(installation.ID())
	declaration := release.Declaration()
	result := make([]mcpserver.Server, 0, len(declaration.Servers))
	for _, declared := range declaration.Servers {
		if err := errors.Join(ctx.Err(), context.Cause(ctx)); err != nil {
			return nil, err
		}
		server, err := descriptorServer(installation, release, declaration, declared, dir, data)
		if err != nil {
			server, err = declaredServer(installation, release, declaration, declared)
			if err != nil {
				return nil, err
			}
		}
		result = append(result, server)
	}
	return result, nil
}

func (r *Releases) Realize(ctx context.Context, installation plugin.Installation, release plugin.Release) (_ plugins.Realization, err error) {
	dir, err := r.Root(release.Digest())
	if err != nil {
		return plugins.Realization{}, fmt.Errorf("pluginpackage: release directory: %w", err)
	}
	lease, verifyErr := r.content.Acquire(ctx, release.Digest())
	if cause := errors.Join(ctx.Err(), context.Cause(ctx)); cause != nil {
		cause = errors.Join(cause, verifyErr)
		if lease.Valid() {
			cause = errors.Join(cause, lease.Close())
		}
		return plugins.Realization{}, cause
	}
	if verifyErr != nil {
		return plugins.Realization{Release: plugins.ReleaseUnavailable}, nil
	}
	defer func() { err = errors.Join(err, lease.Close()) }()
	result := plugins.Realization{Release: plugins.ReleaseAvailable}
	data := r.dataRoot(installation.ID())
	declaration := release.Declaration()
	for _, declared := range declaration.Servers {
		if err := errors.Join(ctx.Err(), context.Cause(ctx)); err != nil {
			return plugins.Realization{}, err
		}
		if _, err := selectedVariant(lease.Snapshot().Manifest, declared.Name); err != nil {
			result.UnavailableBackends = append(result.UnavailableBackends, declared.Name)
			continue
		}
		server, err := descriptorServer(installation, release, declaration, declared, dir, data)
		if err != nil || realizeBackend(ctx, server, declared, dir, data) != nil {
			result.UnavailableBackends = append(result.UnavailableBackends, declared.Name)
		}
	}
	return result, nil
}

func declaredServer(installation plugin.Installation, release plugin.Release, declaration plugin.Declaration, declared plugin.Server) (mcpserver.Server, error) {
	source, err := installation.ServerSource(release, declared.Name)
	if err != nil {
		return mcpserver.Server{}, err
	}
	var connection mcpserver.Connection = mcpserver.StdioConnection{}
	if declared.Transport() == mcpserver.TransportStreamableHTTP {
		connection = mcpserver.HTTPConnection{}
	}
	return mcpserver.Server{Source: source, Name: declared.Name, Connection: connection, Enabled: installation.ServerEnabled(declared.Name), Description: declaration.Description}, nil
}

func (r *Releases) dataRoot(id identity.InstallationID) string {
	return filepath.Join(r.dataDirectory, id.String())
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

func descriptorServer(installation plugin.Installation, release plugin.Release, declaration plugin.Declaration, declared plugin.Server, dir, data string) (mcpserver.Server, error) {
	server, err := declaredServer(installation, release, declaration, declared)
	if err != nil {
		return mcpserver.Server{}, err
	}
	inputs, err := installation.ServerInputs(release, declared.Name)
	if err != nil {
		return mcpserver.Server{}, err
	}
	switch startup := declared.Startup.(type) {
	case plugin.ManagedBackend:
		node, err := hostNode()
		if err != nil {
			return mcpserver.Server{}, err
		}
		process := mcpserver.StdioConnection{
			Command: node.Command(), Args: node.Arguments(filepath.Join(dir, filepath.FromSlash(startup.Entry))),
			ExecutionEnvironment: node.Identity(),
		}
		server.Connection, err = realizeProcess(process, nil, declared, inputs, dir, data)
		if err != nil {
			return mcpserver.Server{}, err
		}
	case plugin.DeclaredConnection:
		switch connection := startup.Connection.(type) {
		case mcpserver.StdioConnection:
			process := mcpserver.StdioConnection{Command: connection.Command}
			if packaged, found := declared.PackagedCommand(); found {
				process.Command = filepath.Join(dir, filepath.FromSlash(packaged))
			}
			for _, arg := range connection.Args {
				process.Args = append(process.Args, plugin.ExpandPlaceholders(arg, dir, data))
			}
			server.Connection, err = realizeProcess(process, connection.Env, declared, inputs, dir, data)
			if err != nil {
				return mcpserver.Server{}, err
			}
		case mcpserver.HTTPConnection:
			headers := maps.Clone(connection.Headers)
			if len(inputs.Headers) > 0 && headers == nil {
				headers = map[string]string{}
			}
			maps.Copy(headers, inputs.Headers)
			server.Connection = mcpserver.HTTPConnection{URL: connection.URL, Authorization: inputs.Authorization, Headers: headers}
		}
	}
	if err := server.Validate(); err != nil {
		return mcpserver.Server{}, fmt.Errorf("pluginpackage: realize server %q: %w", declared.Name, err)
	}
	return server, nil
}

func realizeProcess(process mcpserver.StdioConnection, static map[string]string, declared plugin.Server, inputs plugin.ServerInputs, dir, data string) (mcpserver.Connection, error) {
	process.Env = map[string]string{pluginprocess.RootVariable: dir, pluginprocess.DataVariable: data}
	for key, value := range static {
		process.Env[key] = plugin.ExpandPlaceholders(value, dir, data)
	}
	maps.Copy(process.Env, inputs.Env)
	const searchPath = "PATH"
	if !slices.ContainsFunc(slices.Collect(maps.Keys(process.Env)), func(key string) bool { return strings.EqualFold(key, searchPath) }) {
		process.Env[searchPath] = os.Getenv(searchPath)
	}
	workingDirectory, err := declared.WorkingDirectory()
	if err != nil {
		return nil, err
	}
	boundary, relative := locate(workingDirectory, dir, data)
	process.Dir = filepath.Join(boundary, relative)
	return process, nil
}

func realizeBackend(ctx context.Context, server mcpserver.Server, declared plugin.Server, dir, data string) error {
	process, ok := server.Connection.(mcpserver.StdioConnection)
	if !server.Enabled || !ok {
		return nil
	}
	if _, managed := declared.Startup.(plugin.ManagedBackend); managed {
		node, err := hostNode()
		if err != nil {
			return err
		}
		if process.Command != node.Command() {
			return errors.New("pluginpackage: managed executable differs from distribution")
		}
		if err := node.Verify(ctx); err != nil {
			return err
		}
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

func hostNode() (localruntime.Node, error) {
	executable, err := os.Executable()
	if err != nil {
		return localruntime.Node{}, err
	}
	return localruntime.NodeForExecutable(executable)
}

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
