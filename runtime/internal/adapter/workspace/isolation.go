package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	"github.com/Tangerg/flame/packages/go-sdk/sandbox"
)

type Isolator struct {
	userHome      string
	baseDir       string
	readOnlyPaths []string

	mu         sync.Mutex
	workspaces map[identity.SessionID]*ownedWorkspace
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

func NewIsolator(userHome, baseDir string, readOnlyPaths []string) (*Isolator, error) {
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
		workspaces:    map[identity.SessionID]*ownedWorkspace{},
	}, nil
}

func (i *Isolator) Workspace(ctx context.Context, sessionID identity.SessionID, projectRoot string) (string, error) {
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
	// The tar copy runs outside the lock so slow I/O cannot block another session; Run
	// admission serializes a session's openings, so no second copy for it can appear.
	fresh, err := sandbox.New(ctx, sandbox.Config{
		UserHome:      i.userHome,
		BaseDir:       i.sessionDir(sessionID),
		ReadOnlyPaths: i.readOnlyPaths,
	}, projectRoot)
	if err != nil {
		return "", err
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.closed {
		return "", errors.Join(sandbox.ErrShutdown, fresh.Shutdown())
	}
	if i.workspaces[sessionID] != nil {
		return "", errors.Join(errConcurrentWorkspace, fresh.Shutdown())
	}
	i.workspaces[sessionID] = &ownedWorkspace{workspace: fresh, source: projectRoot}
	return fresh.Path()
}

var errConcurrentWorkspace = errors.New("isolation: concurrent workspace materialization")

func (i *Isolator) lookup(sessionID identity.SessionID) (*ownedWorkspace, bool) {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.workspaces[sessionID], i.closed
}

func (i *Isolator) Discard(sessionID identity.SessionID) error {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.closed {
		return sandbox.ErrShutdown
	}
	return i.discardLocked(sessionID)
}

func (i *Isolator) discardLocked(sessionID identity.SessionID) error {
	if owned := i.workspaces[sessionID]; owned != nil {
		owned.retiring = true
		if err := owned.workspace.Shutdown(); err != nil {
			return err
		}
	}
	if err := os.RemoveAll(i.sessionDir(sessionID)); err != nil {
		return fmt.Errorf("isolation: remove Session scratch namespace: %w", err)
	}
	delete(i.workspaces, sessionID)
	return nil
}

func (i *Isolator) sessionDir(sessionID identity.SessionID) string {
	digest := sha256.Sum256([]byte(sessionID.String()))
	return filepath.Join(i.baseDir, "session-"+hex.EncodeToString(digest[:]))
}

func (i *Isolator) Close() error {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.closed = true
	ids := make([]identity.SessionID, 0, len(i.workspaces))
	for sessionID := range i.workspaces {
		ids = append(ids, sessionID)
	}
	slices.SortFunc(ids, identity.SessionID.Compare)
	var errs []error
	for _, sessionID := range ids {
		if err := i.discardLocked(sessionID); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

var ErrIsolationDisabled = errors.New("isolation: no sandbox directory is configured")

type NoIsolation struct{}

func (NoIsolation) Workspace(context.Context, identity.SessionID, string) (string, error) {
	return "", ErrIsolationDisabled
}

func (NoIsolation) Discard(identity.SessionID) error { return nil }
