package checkpoint

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Tangerg/flame/runtime/internal/infra/filesystem/pathidentity"
)

// A checkpoint cannot archive or restore its own mutable Git metadata. This
// boundary precedes repository creation and pre-restore archiving, including
// when storage is reached through an alias or has not been created yet.
func (s *Store) checkStorageBoundary(ctx context.Context, cwd string) error {
	if cause := context.Cause(ctx); cause != nil {
		return cause
	}
	workspace, err := pathidentity.Resolve("", cwd)
	if err != nil {
		return fmt.Errorf("checkpoint: resolve workspace: %w", err)
	}
	storage, err := pathidentity.Resolve("", s.root)
	if err != nil {
		return fmt.Errorf("checkpoint: resolve storage: %w", err)
	}
	if !strings.EqualFold(filepath.VolumeName(workspace), filepath.VolumeName(storage)) {
		return nil
	}
	for _, pair := range [][2]string{{workspace, storage}, {storage, workspace}} {
		contains, err := pathidentity.Contains(pair[0], pair[1])
		if err != nil {
			return fmt.Errorf("checkpoint: compare workspace and storage: %w", err)
		}
		if contains {
			return fmt.Errorf("%w: workspace overlaps checkpoint storage", ErrUnavailable)
		}
	}
	return nil
}
