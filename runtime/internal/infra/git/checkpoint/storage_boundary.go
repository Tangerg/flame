package checkpoint

import (
	"context"
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
	"github.com/Tangerg/flame/qimu/cancellation"
)

// A checkpoint cannot archive or restore its own mutable Git metadata. This
// boundary precedes repository creation and pre-restore archiving, including
// when storage is reached through an alias or has not been created yet.
func (s *Store) checkStorageBoundary(ctx context.Context, cwd string) error {
	if err := cancellation.Err(ctx); err != nil {
		return err
	}
	workspace, err := filesystem.ResolvePath("", cwd)
	if err != nil {
		return fmt.Errorf("checkpoint: resolve workspace: %w", err)
	}
	storage, err := filesystem.ResolvePath("", s.root)
	if err != nil {
		return fmt.Errorf("checkpoint: resolve storage: %w", err)
	}
	overlaps, err := filesystem.PathsOverlap(workspace, storage)
	if err != nil {
		return fmt.Errorf("checkpoint: compare workspace and storage: %w", err)
	}
	if overlaps {
		return fmt.Errorf("%w: workspace overlaps checkpoint storage", ErrUnavailable)
	}
	return nil
}
