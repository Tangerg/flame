//go:build unix

package checkpoint

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/infra/git/process"
)

func TestCheckpointPredicatesRequireAdmittedGitExit(t *testing.T) {
	raw := exec.CommandContext(t.Context(), "/bin/sh", "-c", "exit 1").Run()
	if exited, ok := errors.AsType[*exec.ExitError](raw); !ok || exited.ExitCode() != 1 {
		t.Fatalf("fixture process error = %v, want exit 1", raw)
	}
	for _, failure := range []error{raw, errors.Join(raw, context.Canceled), errors.Join(raw, process.ErrOutputTooLarge)} {
		if code := gitExitCode(fmt.Errorf("checkpoint observation: %w", failure)); code != -1 {
			t.Errorf("checkpoint predicate admitted process failure %v as exit %d", failure, code)
		}
	}
	admitted := fmt.Errorf("checkpoint query: %w", &gitCommandError{operation: "rev-parse", exitCode: 1})
	if code := gitExitCode(admitted); code != 1 {
		t.Errorf("admitted Git predicate exit = %d, want 1", code)
	}
}
