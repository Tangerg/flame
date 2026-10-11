// Package workspace translates filesystem and Git capabilities into workspace
// observations, confined file access, and per-session checkpoint lifecycles.
package workspace

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/application/agent/sessions"
	"github.com/Tangerg/flame/runtime/internal/infra/git"
	"github.com/Tangerg/flame/runtime/internal/infra/git/checkpoint"
)

// Checkpoints owns the per-session file-checkpoint lifecycle over a shadow-git
// store. An unconfigured store skips snapshots and reports restores unavailable.
type Checkpoints struct {
	store *checkpoint.Store // nil when git is unavailable / no dir
}

// NewCheckpoints builds the checkpoint adapter. File checkpoints are enabled
// only when the git binary is present and checkpointDir is non-empty; otherwise
// the store is nil and snapshot/restore degrade to the unavailable path.
func NewCheckpoints(checkpointDir string) *Checkpoints {
	var store *checkpoint.Store
	if checkpointDir != "" && git.Available() {
		store = checkpoint.NewStore(checkpointDir)
	}
	return &Checkpoints{store: store}
}

// CheckpointsEnabled reports whether file checkpoints are available — backs
// the features.checkpoints flag. An unconfigured checkpoint directory is a
// user-disabled feature, not a missing collaborator.
func (c *Checkpoints) CheckpointsEnabled() bool {
	return c.store != nil
}

// Snapshot anchors sessionID's working tree (at cwd) under runID so a later
// Restore can revert to it. A disabled store is a no-op. Operational failures
// remain observable; the caller owns best-effort maintenance policy.
func (c *Checkpoints) Snapshot(ctx context.Context, sessionID, cwd, runID string) error {
	if !c.CheckpointsEnabled() {
		return nil
	}
	// Only checkpoint a real git repo — state tracking requires git.
	// A repo's own .gitignore bounds what the whole-tree `git add` stages; a
	// non-repo dir (e.g. a session opened on the home directory) has no such
	// bound, so snapshotting it would try to stage the entire tree — minutes of
	// `git add` on millions of files. Non-git projects get no file checkpoint,
	// by design: file rollback is a git-shaped feature, only safe where a
	// .gitignore scopes the work tree.
	repository, err := git.IsRepo(ctx, cwd)
	if err != nil {
		return fmt.Errorf("checkpoints: inspect workspace: %w", err)
	}
	if !repository {
		return nil
	}
	return c.store.Snapshot(ctx, sessionID, cwd, runID)
}

// Restore checks out sessionID's working tree (at cwd) to the runID snapshot. A
// disabled store or missing snapshot surfaces as [sessions.ErrCheckpointUnavailable]; a
// failed checkout that may have changed part of the tree surfaces as
// [sessions.ErrCheckpointRestoreIncomplete].
func (c *Checkpoints) Restore(ctx context.Context, sessionID, cwd, runID string) error {
	if !c.CheckpointsEnabled() {
		return sessions.ErrCheckpointUnavailable
	}
	err := c.store.Restore(ctx, sessionID, cwd, runID)
	switch {
	case errors.Is(err, checkpoint.ErrUnavailable):
		return fmt.Errorf("%w: %w", sessions.ErrCheckpointUnavailable, err)
	case errors.Is(err, checkpoint.ErrConflict):
		return fmt.Errorf("%w: %w", sessions.ErrCheckpointConflict, err)
	case errors.Is(err, checkpoint.ErrRestoreIncomplete):
		return fmt.Errorf("%w: %w", sessions.ErrCheckpointRestoreIncomplete, err)
	default:
		return err
	}
}

// DropSession removes a session's shadow repo (on session delete).
// Best-effort no-op when checkpoints are disabled.
func (c *Checkpoints) DropSession(sessionID string) error {
	if !c.CheckpointsEnabled() {
		return nil
	}
	return c.store.DropSession(sessionID)
}
