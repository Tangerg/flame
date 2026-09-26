package checkpoint

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestStoreSnapshotRefusesStorageOverlapBeforeWriting(t *testing.T) {
	for _, layout := range []string{"storage inside workspace", "workspace inside storage", "same directory", "storage alias inside workspace"} {
		t.Run(layout, func(t *testing.T) {
			base := t.TempDir()
			cwd, storage := base, filepath.Join(base, "state", "checkpoints")
			switch layout {
			case "workspace inside storage":
				cwd, storage = filepath.Join(base, "workspace"), base
			case "same directory":
				storage = base
			case "storage alias inside workspace":
				alias := filepath.Join(t.TempDir(), "alias")
				if err := os.Symlink(base, alias); err != nil {
					t.Fatal(err)
				}
				storage = filepath.Join(alias, "state", "checkpoints")
			}
			if err := os.MkdirAll(cwd, 0o755); err != nil {
				t.Fatal(err)
			}
			write(t, cwd, "material.txt", "current workspace material")
			write(t, base, "runtime-state.db", "current runtime state")
			s := NewStore(storage)
			if err := s.Snapshot(t.Context(), "session", cwd, "boundary"); !errors.Is(err, ErrUnavailable) {
				t.Fatalf("snapshot with storage overlap = %v, want unavailable", err)
			}
			if _, err := os.Stat(s.sessionDir("session")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("rejected snapshot created a Session repository: %v", err)
			}
			if got := read(t, cwd, "material.txt"); got != "current workspace material" {
				t.Fatalf("rejected snapshot changed workspace material to %q", got)
			}
			if got := read(t, base, "runtime-state.db"); got != "current runtime state" {
				t.Fatalf("rejected snapshot changed Runtime state to %q", got)
			}
		})
	}
}

func TestStoreRestoreRefusesStorageOverlapBeforeArchiving(t *testing.T) {
	s, cwd := newTestStore(t)
	write(t, cwd, "material.txt", "checkpoint material")
	if err := s.Snapshot(t.Context(), "session", cwd, "boundary"); err != nil {
		t.Fatal(err)
	}
	head, err := s.git(t.Context(), s.gitDir("session", cwd), cwd, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	storage := filepath.Join(cwd, "runtime-state")
	if err := os.Rename(s.root, storage); err != nil {
		t.Fatal(err)
	}
	s = NewStore(storage)
	write(t, cwd, "material.txt", "current workspace material")
	write(t, storage, "runtime-state.db", "current runtime state")
	if err := s.Restore(t.Context(), "session", cwd, "boundary"); !errors.Is(err, ErrUnavailable) || errors.Is(err, ErrRestoreIncomplete) {
		t.Fatalf("restore with storage overlap = %v, want pre-archive unavailable", err)
	}
	if got := read(t, cwd, "material.txt"); got != "current workspace material" {
		t.Fatalf("rejected restore changed workspace material to %q", got)
	}
	if got := read(t, storage, "runtime-state.db"); got != "current runtime state" {
		t.Fatalf("rejected restore changed Runtime state to %q", got)
	}
	if got, err := s.git(t.Context(), s.gitDir("session", cwd), cwd, "rev-parse", "HEAD"); err != nil || got != head {
		t.Fatalf("rejected restore changed checkpoint history: HEAD = %q, error = %v", got, err)
	}
}
