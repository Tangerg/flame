// Package isolation owns the per-session sandbox working copies that back an
// isolated run: a [session.Session] marked Isolated runs its tools inside a
// throwaway tar-copy of its project directory (the sandbox Workspace) rather
// than the real tree, so file changes never touch the project and the jailed
// shell cannot reach the network. It is the adapter that activates the C7
// isolated-copy sandbox — wrapping infra/process/sandbox behind the narrow ports the
// runs coordinator (resolve) and the session-delete cascade (discard) consume.
package isolation

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"sync"

	"github.com/Tangerg/flame/runtime/internal/infra/process/sandbox"
)

// Isolator holds one sandbox Workspace per isolated session. The copy is
// created lazily on the session's first isolated run and reused across its
// Runs (so the agent's work accumulates), then destroyed when the session is
// deleted or the runtime stops. The copy is a scratch tree: it is never
// snapshotted or written back — isolation means the project is left untouched.
// Safe for concurrent use.
type Isolator struct {
	userHome      string
	baseDir       string
	readOnlyPaths []string

	mu         sync.Mutex
	workspaces map[string]*ownedWorkspace
	closed     bool
}

type ownedWorkspace struct {
	workspace *sandbox.Workspace
	source    string
	retiring  bool
}

var (
	errWorkspaceRetiring = errors.New("isolation: workspace cleanup is incomplete")
	errWorkspaceChanged  = errors.New("isolation: session workspace changed without retiring its copy")
)

// New builds an isolator rooting its ephemeral copies under baseDir (a trusted
// path owned by the runtime). readOnlyPaths re-opens toolchain roots below the
// hidden home for the jailed shell.
func New(userHome, baseDir string, readOnlyPaths []string) (*Isolator, error) {
	if userHome == "" {
		return nil, errors.New("isolation: user home is required")
	}
	if !filepath.IsAbs(userHome) {
		return nil, errors.New("isolation: user home must be absolute")
	}
	if baseDir == "" {
		return nil, errors.New("isolation: base directory is required")
	}
	if !filepath.IsAbs(baseDir) {
		return nil, errors.New("isolation: base directory must be absolute")
	}
	for i, path := range readOnlyPaths {
		if path != "" && !filepath.IsAbs(path) {
			return nil, fmt.Errorf("isolation: read-only path %d must be absolute", i)
		}
	}
	return &Isolator{
		userHome:      userHome,
		baseDir:       baseDir,
		readOnlyPaths: slices.Clone(readOnlyPaths),
		workspaces:    map[string]*ownedWorkspace{},
	}, nil
}

// Workspace returns the isolated working-copy directory for sessionID, creating
// it from projectRoot (a tar copy of the real project) on first use and reusing
// it thereafter. It fails closed with [sandbox.ErrUnavailable] when the host has
// no isolation backend, so an isolated run is refused rather than run
// unconfined.
func (i *Isolator) Workspace(ctx context.Context, sessionID, projectRoot string) (string, error) {
	owned, closed := i.lookup(sessionID)
	if closed {
		return "", sandbox.ErrShutdown
	}
	if owned != nil {
		if owned.retiring {
			return "", errWorkspaceRetiring
		}
		if owned.source != projectRoot {
			return "", errWorkspaceChanged
		}
		return owned.workspace.Path()
	}
	// Materialize the copy OUTSIDE the lock: a tar copy is slow I/O and must not
	// block another session's Workspace/Discard. A session's runs are serialized
	// by admission, so the same session cannot race here; the store-back below
	// still resolves a race defensively (discarding the losing copy).
	fresh, err := sandbox.New(ctx, sandbox.Config{
		UserHome:      i.userHome,
		BaseDir:       i.baseDir,
		ReadOnlyPaths: i.readOnlyPaths,
	}, projectRoot)
	if err != nil {
		return "", err
	}
	i.mu.Lock()
	if i.closed {
		i.mu.Unlock()
		return "", errors.Join(sandbox.ErrShutdown, fresh.Shutdown())
	}
	if existing := i.workspaces[sessionID]; existing != nil {
		if err := fresh.Shutdown(); err != nil {
			i.mu.Unlock()
			return "", fmt.Errorf("isolation: discard redundant workspace: %w", err)
		}
		if existing.retiring {
			i.mu.Unlock()
			return "", errWorkspaceRetiring
		}
		if existing.source != projectRoot {
			i.mu.Unlock()
			return "", errWorkspaceChanged
		}
		path, err := existing.workspace.Path()
		i.mu.Unlock()
		return path, err
	}
	i.workspaces[sessionID] = &ownedWorkspace{workspace: fresh, source: projectRoot}
	i.mu.Unlock()
	return fresh.Path()
}

func (i *Isolator) lookup(sessionID string) (*ownedWorkspace, bool) {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.workspaces[sessionID], i.closed
}

// Discard destroys a session's isolated working copy. Idempotent: a session
// that never ran isolated (no workspace) is a no-op. A failed destruction
// leaves the copy poisoned and retryable; Workspace will not expose stale
// scratch state while cleanup is incomplete.
func (i *Isolator) Discard(sessionID string) error {
	i.mu.Lock()
	defer i.mu.Unlock()
	owned := i.workspaces[sessionID]
	if owned == nil {
		return nil
	}
	owned.retiring = true
	if err := owned.workspace.Shutdown(); err != nil {
		return err
	}
	delete(i.workspaces, sessionID)
	return nil
}

// Close destroys every live working copy — the process-shutdown closer. It
// joins every teardown error.
func (i *Isolator) Close() error {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.closed = true
	ids := make([]string, 0, len(i.workspaces))
	for sessionID := range i.workspaces {
		ids = append(ids, sessionID)
	}
	slices.Sort(ids)
	var errs []error
	for _, sessionID := range ids {
		owned := i.workspaces[sessionID]
		owned.retiring = true
		if err := owned.workspace.Shutdown(); err != nil {
			errs = append(errs, err)
			continue
		}
		delete(i.workspaces, sessionID)
	}
	return errors.Join(errs...)
}
