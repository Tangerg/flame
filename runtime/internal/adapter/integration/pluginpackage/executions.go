package pluginpackage

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"slices"

	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	mcpapp "github.com/Tangerg/flame/runtime/internal/application/integration/mcp"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"

	"github.com/Tangerg/frame/distribution"
	framevalue "github.com/Tangerg/frame/value"
)

// Connection transfers the original content lease to the real process owner.
// There is no copied execution tree, second release digest or process cache.
func (r *Releases) Connection(ctx context.Context, installation plugin.Installation, release plugin.Release, name mcpserver.ServerName) (connection mcpapp.Launch, found bool, err error) {
	server, found, err := r.Server(ctx, installation, release, name, plugins.Declared)
	if err != nil || !found || !server.Enabled {
		return mcpapp.Launch{Server: server}, found, err
	}
	lease, err := r.content.Acquire(ctx, release.Digest())
	if err != nil {
		return mcpapp.Launch{}, false, errors.Join(plugin.ErrUnavailable, err)
	}
	transferred := false
	defer func() {
		if !transferred {
			err = errors.Join(err, lease.Close())
		}
		if err != nil {
			connection = mcpapp.Launch{}
			found = false
		}
	}()
	if server.Transport() != mcpserver.TransportStdio {
		return mcpapp.Launch{Server: server}, true, nil
	}
	directory, err := lease.Directory()
	if err != nil {
		return mcpapp.Launch{}, false, err
	}
	declaration := release.Declaration()
	declared := declaration.Servers[slices.IndexFunc(declaration.Servers, func(s plugin.Server) bool { return s.Name == name })]
	variant, err := selectedVariant(lease.Snapshot().Manifest, declared.Name)
	if err != nil {
		return mcpapp.Launch{}, false, err
	}
	if err := r.prepareBackend(installation.ID(), declared); err != nil {
		return mcpapp.Launch{}, false, fmt.Errorf("%w: prepare server %q backend: %w", plugin.ErrUnavailable, name, err)
	}
	data := r.dataRoot(installation.ID())
	executable, err := descriptorServer(installation, release, declaration, declared, directory, data)
	if err != nil {
		return mcpapp.Launch{}, false, err
	}
	if err := realizeBackend(ctx, executable, declared, directory, data); err != nil {
		return mcpapp.Launch{}, false, fmt.Errorf("%w: server %q backend: %w", plugin.ErrUnavailable, name, err)
	}
	expected, err := expectedExecutable(declared, variant, lease.Snapshot().Manifest)
	if err != nil {
		return mcpapp.Launch{}, false, err
	}
	process, ok := executable.Connection.(mcpserver.StdioConnection)
	if !ok {
		return mcpapp.Launch{}, false, fmt.Errorf("%w: server %q realized no process", plugin.ErrInvalid, name)
	}
	transferred = true
	return mcpapp.Launch{Server: server, Stdio: &mcpapp.Stdio{Command: process.Command, ExpectedExecutable: expected, Args: process.Args, Env: process.Env, Dir: process.Dir}, Retire: lease}, true, nil
}

// expectedExecutable is the digest the launched process must have: the pinned
// interpreter of a managed backend, or the packaged executable of a native one.
func expectedExecutable(declared plugin.Server, variant distribution.Variant, manifest distribution.Manifest) (framevalue.Digest, error) {
	if _, managed := declared.Startup.(plugin.ManagedBackend); managed {
		node, err := hostNode()
		if err != nil {
			return framevalue.Digest{}, err
		}
		expected, err := framevalue.ParseDigest(node.ExecutableDigest())
		if err != nil {
			return framevalue.Digest{}, err
		}
		if variant.Runtime == nil || expected != variant.Runtime.Digest {
			return framevalue.Digest{}, fmt.Errorf("%w: installed interpreter differs from its original runtime pin", plugin.ErrUnavailable)
		}
		return expected, nil
	}
	command, packaged := declared.PackagedCommand()
	if !packaged {
		return framevalue.Digest{}, fmt.Errorf("%w: installed process entry must be a packaged executable", plugin.ErrInvalid)
	}
	for _, artifact := range manifest.Artifacts {
		if artifact.Path == command && artifact.Executable {
			return artifact.Digest, artifact.Digest.Validate()
		}
	}
	return framevalue.Digest{}, fmt.Errorf("%w: packaged executable %q is missing from the release", plugin.ErrInvalid, command)
}

func selectedVariant(manifest distribution.Manifest, server mcpserver.ServerName) (distribution.Variant, error) {
	for _, unit := range manifest.Units {
		if unit.ID != plugins.BackendUnit(server) || unit.Profile != pluginmcp.ProcessProfile {
			continue
		}
		for _, variant := range unit.Variants {
			if variant.OS == runtime.GOOS && variant.Arch == runtime.GOARCH {
				return variant, nil
			}
		}
	}
	return distribution.Variant{}, fmt.Errorf("%w: package has no process entry for %s/%s", plugin.ErrUnavailable, runtime.GOOS, runtime.GOARCH)
}
