package runs

import (
	"fmt"
	"strings"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
)

func validateCheckpointSessionScope(
	checkpoint run.Checkpoint,
	sess session.Session,
) error {
	if checkpoint.Scope().WorkspaceCWD != sess.Workspace().Path() || checkpoint.Scope().Isolated != sess.Isolated() {
		return fmt.Errorf("%w: checkpoint workspace scope differs from Session", ErrExecutorStateLost)
	}
	if sess.Isolated() {
		if strings.TrimSpace(checkpoint.Scope().CWD) == "" {
			return fmt.Errorf("%w: isolated checkpoint working directory is empty", ErrExecutorStateLost)
		}
		return nil
	}
	if checkpoint.Scope().CWD != sess.Workspace().Path() {
		return fmt.Errorf("%w: checkpoint working directory differs from Session", ErrExecutorStateLost)
	}
	return nil
}
