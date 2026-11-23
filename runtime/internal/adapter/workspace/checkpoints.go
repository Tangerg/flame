package workspace

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	"github.com/Tangerg/flame/runtime/internal/application/agent/sessions"
	"github.com/Tangerg/flame/runtime/internal/infra/git"
	"github.com/Tangerg/flame/runtime/internal/infra/git/checkpoint"
)

type Checkpoints struct {
	store *checkpoint.Store
}

func NewCheckpoints(checkpointDir string) *Checkpoints {
	var store *checkpoint.Store
	if checkpointDir != "" && git.Available() {
		store = checkpoint.NewStore(checkpointDir)
	}
	return &Checkpoints{store: store}
}

func (c *Checkpoints) CheckpointsEnabled() bool {
	return c.store != nil
}

func (c *Checkpoints) Snapshot(ctx context.Context, sessionID identity.SessionID, cwd string, runID identity.RunID) error {
	if !c.CheckpointsEnabled() {
		return nil
	}
	// Only a real git repo is checkpointed: its .gitignore bounds the whole-tree git
	// add, while a non-repo directory such as home would stage millions of files.
	repository, err := git.IsRepo(ctx, cwd)
	if err != nil {
		return fmt.Errorf("checkpoints: inspect workspace: %w", err)
	}
	if !repository {
		return nil
	}
	return c.store.Snapshot(ctx, sessionID.String(), cwd, runID.String())
}

func (c *Checkpoints) Restore(ctx context.Context, sessionID identity.SessionID, cwd string, runID identity.RunID) error {
	if !c.CheckpointsEnabled() {
		return sessions.ErrCheckpointUnavailable
	}
	err := c.store.Restore(ctx, sessionID.String(), cwd, runID.String())
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

func (c *Checkpoints) DropSession(ctx context.Context, sessionID identity.SessionID) error {
	if !c.CheckpointsEnabled() {
		return nil
	}
	return c.store.DropSession(ctx, sessionID.String())
}
