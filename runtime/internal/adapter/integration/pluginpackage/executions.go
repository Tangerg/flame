package pluginpackage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	mcpapp "github.com/Tangerg/flame/runtime/internal/application/integration/mcp"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/fingerprint"
	"github.com/Tangerg/flame/runtime/internal/infra/advisorylock"
)

// executionContent is an immutable projection of admitted bytes, shared by
// connections of one digest. users counts resource claims, never installation
// admission or executable-manifest dependencies. MCP owns each claim's lifetime.
type executionContent struct {
	ready chan struct{}
	tree  *executionTree
	err   error
	users int
}

type executionTree struct {
	path  string
	root  *os.Root
	lease *advisorylock.Lease
}

func (t *executionTree) discard() error {
	return errors.Join(t.root.Close(), removeSealed(t.path), t.lease.Release())
}

type executionLease struct {
	path  string
	close func() error
}

func (l *executionLease) Close() error { return l.close() }

func (r *Releases) executionDirectory() string {
	return filepath.Join(filepath.Dir(r.directory), "executions")
}

// ReclaimExecutions collects crash leftovers. A per-tree directory lease
// protects every live copy, including copies held by another Runtime using the
// same data directory. The namespace lease makes creation and collection one
// atomic handoff; it is never held while copying package bytes.
func (r *Releases) ReclaimExecutions(ctx context.Context) (err error) {
	dir := r.executionDirectory()
	if info, err := os.Lstat(dir); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	} else if !info.IsDir() {
		return errors.New("pluginpackage: execution namespace is not a directory")
	}
	namespace, err := advisorylock.AcquireDirectory(ctx, dir)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, namespace.Release()) }()
	directory, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, directory.Close()) }()
	for {
		entries, readErr := directory.ReadDir(reclaimBatch)
		for _, entry := range entries {
			if cause := context.Cause(ctx); cause != nil {
				return cause
			}
			if !entry.IsDir() || !strings.HasPrefix(entry.Name(), ".stage-") {
				continue
			}
			path := filepath.Join(dir, entry.Name())
			lease, err := advisorylock.TryDirectory(path)
			if errors.Is(err, advisorylock.ErrContended) || errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err != nil {
				return err
			}
			if err := errors.Join(removeSealed(path), lease.Release()); err != nil {
				return err
			}
		}
		if errors.Is(readErr, io.EOF) {
			return nil
		}
		if readErr != nil {
			return readErr
		}
	}
}

func (r *Releases) Connection(ctx context.Context, installation *plugin.Installation, release plugin.Release, name mcpserver.ServerName) (connection mcpapp.Launch, found bool, err error) {
	server, found, err := r.Server(ctx, installation, release, name, plugins.Declared)
	if err != nil || !found || !server.Enabled {
		return mcpapp.Launch{Server: server}, found, err
	}
	root, err := r.currentRoot(ctx, release.Digest())
	if err != nil {
		return mcpapp.Launch{}, false, errors.Join(plugin.ErrUnavailable, err)
	}
	var lease *executionLease
	defer func() {
		err = errors.Join(err, root.Close())
		if err != nil {
			if lease != nil {
				err = errors.Join(err, lease.Close())
			}
			connection, found = mcpapp.Launch{}, false
		}
	}()
	if server.Transport != mcpserver.TransportStdio {
		return mcpapp.Launch{Server: server}, true, nil
	}
	lease, err = r.execution(ctx, release.Digest(), root)
	if err != nil {
		return mcpapp.Launch{}, false, fmt.Errorf("%w: execution content: %w", plugin.ErrUnavailable, err)
	}
	declaration := release.Declaration()
	declared := declaration.Servers[slices.IndexFunc(declaration.Servers, func(s plugin.Server) bool { return s.Name == name })]
	if err := r.prepareBackend(installation.ID(), declared); err != nil {
		return mcpapp.Launch{}, false, fmt.Errorf("%w: prepare server %q backend: %w", plugin.ErrUnavailable, name, err)
	}
	data := r.dataRoot(installation.ID())
	executable, err := descriptorServer(installation, release, declaration, declared, lease.path, data)
	if err != nil {
		return mcpapp.Launch{}, false, err
	}
	if err := realizeBackend(executable, declared, lease.path, data); err != nil {
		return mcpapp.Launch{}, false, fmt.Errorf("%w: server %q backend: %w", plugin.ErrUnavailable, name, err)
	}
	return mcpapp.Launch{Server: server, Stdio: &mcpapp.Stdio{Command: executable.Command, Args: executable.Args, Env: executable.Env, Dir: executable.Dir}, Retire: lease}, true, nil
}

func (r *Releases) execution(ctx context.Context, digest fingerprint.Digest, source *os.Root) (*executionLease, error) {
	r.executionMu.Lock()
	content := r.executions[digest]
	build := content == nil
	if build {
		content = &executionContent{ready: make(chan struct{})}
		r.executions[digest] = content
	}
	content.users++
	r.executionMu.Unlock()
	if build {
		tree, err := r.copyExecution(ctx, source, digest)
		r.executionMu.Lock()
		content.tree, content.err = tree, err
		if err != nil {
			delete(r.executions, digest)
		}
		close(content.ready)
		r.executionMu.Unlock()
	}
	select {
	case <-content.ready:
		if content.err != nil {
			return nil, content.err
		}
	case <-ctx.Done():
		return nil, errors.Join(context.Cause(ctx), r.releaseExecution(digest, content))
	}
	if cause := context.Cause(ctx); cause != nil {
		return nil, errors.Join(cause, r.releaseExecution(digest, content))
	}
	return &executionLease{path: content.tree.path, close: sync.OnceValue(func() error { return r.releaseExecution(digest, content) })}, nil
}

func (r *Releases) copyExecution(ctx context.Context, source *os.Root, digest fingerprint.Digest) (_ *executionTree, err error) {
	dir := r.executionDirectory()
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return nil, err
	}
	if resolved != dir {
		return nil, errors.New("pluginpackage: execution directory is redirected")
	}
	namespace, err := advisorylock.AcquireDirectory(ctx, dir)
	if err != nil {
		return nil, err
	}
	staged, err := newStaging(dir)
	if err != nil {
		return nil, errors.Join(err, namespace.Release())
	}
	lease, err := advisorylock.TryDirectory(staged.path)
	if err != nil {
		return nil, errors.Join(err, staged.discard(), namespace.Release())
	}
	tree := &executionTree{path: staged.path, root: staged.root, lease: lease}
	defer func() {
		if err != nil {
			err = errors.Join(err, tree.discard())
		}
	}()
	if err := namespace.Release(); err != nil {
		return nil, err
	}
	if err := staged.copyDirectory(ctx, source); err != nil {
		return nil, err
	}
	if err := staged.seal(); err != nil {
		return nil, err
	}
	actual, _, err := treeDigest(ctx, tree.root)
	if err != nil {
		return nil, err
	}
	if actual != digest {
		return nil, errors.New("pluginpackage: execution content integrity mismatch")
	}
	return tree, nil
}

func (r *Releases) releaseExecution(digest fingerprint.Digest, content *executionContent) error {
	r.executionMu.Lock()
	content.users--
	last := content.users == 0 && content.tree != nil
	if last {
		delete(r.executions, digest)
	}
	r.executionMu.Unlock()
	if last {
		return content.tree.discard()
	}
	return nil
}
