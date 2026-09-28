package workspace

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestSnapshotDistinguishesNonRepositoryFromBrokenRepository(t *testing.T) {
	checkpoints := NewCheckpoints(t.TempDir())
	if !checkpoints.CheckpointsEnabled() {
		t.Skip("git executable is required")
	}
	cwd := t.TempDir()
	if err := checkpoints.Snapshot(context.Background(), "session", cwd, "run"); err != nil {
		t.Fatalf("non-repository: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cwd, ".git"), []byte("not a gitdir declaration"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := checkpoints.Snapshot(context.Background(), "session", cwd, "run"); err == nil {
		t.Fatal("broken repository inspection was reported as success")
	}
}
