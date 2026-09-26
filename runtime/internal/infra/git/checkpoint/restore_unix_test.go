//go:build unix

package checkpoint

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestStoreRestoreProtectsUnarchivedSpecialNodes(t *testing.T) {
	for _, directory := range []bool{false, true} {
		t.Run(map[bool]string{false: "file replaced by pipe", true: "directory contains pipe"}[directory], func(t *testing.T) {
			s, cwd := newTestStore(t)
			write(t, cwd, "entry", "checkpoint")
			if err := s.Snapshot(t.Context(), "session", cwd, "boundary"); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(cwd, "entry")
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if directory {
				if err := os.Mkdir(path, 0o755); err != nil {
					t.Fatal(err)
				}
				write(t, path, "archived", "current")
				path = filepath.Join(path, "unarchived")
			}
			if err := syscall.Mkfifo(path, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := s.Restore(t.Context(), "session", cwd, "boundary"); !errors.Is(err, ErrConflict) {
				t.Fatalf("restore = %v, want special-node conflict", err)
			}
			if info, err := os.Lstat(path); err != nil || info.Mode()&os.ModeNamedPipe == 0 {
				t.Fatalf("unarchived pipe changed: info = %v, error = %v", info, err)
			}
		})
	}
}
