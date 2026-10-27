package pluginpackage

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"slices"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/qimu"
	"github.com/Tangerg/flame/qimu/distribution"
	mcpapp "github.com/Tangerg/flame/runtime/internal/application/integration/mcp"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
)

// Connection transfers the original content lease to the real process owner.
// There is no copied execution tree, second release digest or process cache.
func (r *Releases) Connection(ctx context.Context, installation *plugin.Installation, release plugin.Release, name mcpserver.ServerName) (connection mcpapp.Launch, found bool, err error) {
	server, found, err := r.Server(ctx, installation, release, name, plugins.Declared)
	if err != nil || !found || !server.Enabled {
		return mcpapp.Launch{Server: server}, found, err
	}
	lease, err := r.content.Acquire(ctx, qimu.Digest(release.Digest().String()))
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
	if server.Transport != mcpserver.TransportStdio {
		return mcpapp.Launch{Server: server}, true, nil
	}
	directory, err := lease.Directory()
	if err != nil {
		return mcpapp.Launch{}, false, err
	}
	declaration := release.Declaration()
	declared := declaration.Servers[slices.IndexFunc(declaration.Servers, func(s plugin.Server) bool { return s.Name == name })]
	variant, err := selectedVariant(lease.Release().Manifest, declared.Name)
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
	var expected identity.Digest
	if declared.Managed != nil {
		node, nodeErr := hostNode()
		if nodeErr != nil {
			return mcpapp.Launch{}, false, nodeErr
		}
		expected, err = identity.ParseDigest(node.ExecutableDigest())
		if variant.Runtime == nil || qimu.Digest(expected.String()) != variant.Runtime.Digest {
			return mcpapp.Launch{}, false, fmt.Errorf("%w: installed interpreter differs from its original runtime pin", plugin.ErrUnavailable)
		}
	} else {
		command, packaged := declared.PackagedCommand()
		if !packaged {
			return mcpapp.Launch{}, false, fmt.Errorf("%w: installed process entry must be a packaged executable", plugin.ErrInvalid)
		}
		for _, artifact := range lease.Release().Manifest.Artifacts {
			if artifact.Path == command && artifact.Executable {
				expected, err = identity.ParseDigest(artifact.Digest.String())
				break
			}
		}
		if err == nil {
			err = expected.Validate()
		}
	}
	if err != nil {
		return mcpapp.Launch{}, false, err
	}
	transferred = true
	return mcpapp.Launch{Server: server, Stdio: &mcpapp.Stdio{Command: executable.Command, ExpectedExecutable: expected, Args: executable.Args, Env: executable.Env, Dir: executable.Dir}, Retire: lease}, true, nil
}

func selectedVariant(manifest distribution.Manifest, server mcpserver.ServerName) (distribution.Variant, error) {
	for _, unit := range manifest.Units {
		if unit.ID != plugins.BackendUnit(server) || unit.Profile != qimu.MCPProcess {
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
