//go:build unix

package workspace_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/adapter/workspace"
	"github.com/Tangerg/flame/runtime/internal/application/agent/sessions"
	"github.com/Tangerg/flame/runtime/internal/infra/git/checkpoint"
)

func TestSessionCheckpointUnavailableRetainsRefusalCause(t *testing.T) {
	cwd := t.TempDir()
	checkpoints := workspace.NewCheckpoints(cwd)
	if !checkpoints.CheckpointsEnabled() {
		t.Skip("git not installed; checkpoint restore requires git")
	}
	err := checkpoints.Restore(t.Context(), "session", cwd, "boundary")
	if !errors.Is(err, sessions.ErrCheckpointUnavailable) || !errors.Is(err, checkpoint.ErrUnavailable) {
		t.Fatalf("restore refused = %v, want application category and storage cause", err)
	}
	if !strings.Contains(err.Error(), "workspace overlaps checkpoint storage") {
		t.Fatalf("restore refused = %v, want original boundary reason", err)
	}
}

func TestSessionCheckpointRestorePreservesGitFailures(t *testing.T) {
	for _, operation := range []string{"checkout", "rev-parse"} {
		t.Run(operation, func(t *testing.T) {
			realGit, err := exec.LookPath("git")
			if err != nil {
				t.Skip("git not installed; checkpoint restore requires git")
			}
			cwd := t.TempDir()
			if output, err := exec.Command(realGit, "-C", cwd, "init", "-q").CombinedOutput(); err != nil {
				t.Fatalf("git init: %v: %s", err, output)
			}
			if err := os.WriteFile(filepath.Join(cwd, "material.txt"), []byte("checkpoint\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			checkpoints := workspace.NewCheckpoints(t.TempDir())
			if err := checkpoints.Snapshot(t.Context(), "session", cwd, "boundary"); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(cwd, "material.txt"), []byte("current\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			bin := t.TempDir()
			const script = `#!/bin/sh
if [ "$1" = "$FLAME_CHECKPOINT_TEST_OPERATION" ]; then exit 73; fi
exec "$FLAME_CHECKPOINT_TEST_GIT" "$@"
`
			if err := os.WriteFile(filepath.Join(bin, "git"), []byte(script), 0o700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("FLAME_CHECKPOINT_TEST_GIT", realGit)
			t.Setenv("FLAME_CHECKPOINT_TEST_OPERATION", operation)
			t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
			err = checkpoints.Restore(t.Context(), "session", cwd, "boundary")
			incomplete := operation == "checkout"
			if errors.Is(err, sessions.ErrCheckpointRestoreIncomplete) != incomplete || errors.Is(err, checkpoint.ErrRestoreIncomplete) != incomplete {
				t.Fatalf("restore lost its incomplete classification: %v", err)
			}
			if errors.Is(err, sessions.ErrCheckpointUnavailable) || errors.Is(err, checkpoint.ErrUnavailable) {
				t.Fatalf("restore disguised a git failure as a missing checkpoint: %v", err)
			}
			exited, found := errors.AsType[interface {
				error
				ExitCode() int
			}](err)
			if !found || exited.ExitCode() != 73 {
				t.Fatalf("restore lost the observed git exit code: %v", err)
			}
			if material, err := os.ReadFile(filepath.Join(cwd, "material.txt")); err != nil || string(material) != "current\n" {
				t.Fatalf("rejected restore changed files: %q, %v", material, err)
			}
		})
	}
}
