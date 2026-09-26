package checkpoint

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestStoreRestorePreservesUnarchivedMaterial(t *testing.T) {
	for _, shape := range []string{"oversized file", "previously untracked oversized file", "ignored file", "ignored directory contents", "oversized directory contents"} {
		t.Run(shape, func(t *testing.T) {
			s, cwd := newTestStore(t)
			write(t, cwd, "entry", "checkpoint content")
			write(t, cwd, "sibling", "checkpoint sibling")
			if err := s.Snapshot(t.Context(), "session", cwd, "boundary"); err != nil {
				t.Fatal(err)
			}
			write(t, cwd, "sibling", "current sibling")
			path := filepath.Join(cwd, "entry")
			material := strings.Repeat("unarchived", maxCheckpointFileSize/10+1)
			if shape == "ignored file" {
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				write(t, cwd, ".gitignore", "entry\n")
				if err := s.Snapshot(t.Context(), "session", cwd, "later"); err != nil {
					t.Fatal(err)
				}
				material = "unarchived ignored file"
			}
			if strings.HasSuffix(shape, "directory contents") {
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(path, 0o755); err != nil {
					t.Fatal(err)
				}
				if shape == "ignored directory contents" {
					write(t, cwd, ".gitignore", "entry/\n")
					material = "unarchived directory contents"
				}
				path = filepath.Join(path, "unarchived.txt")
			}
			if err := os.WriteFile(path, []byte(material), 0o644); err != nil {
				t.Fatal(err)
			}
			if shape == "previously untracked oversized file" {
				if err := s.Snapshot(t.Context(), "session", cwd, "later"); err != nil {
					t.Fatal(err)
				}
			}
			if err := s.Restore(t.Context(), "session", cwd, "boundary"); !errors.Is(err, ErrConflict) || errors.Is(err, ErrRestoreIncomplete) {
				t.Errorf("restore = %v, want pre-checkout conflict", err)
			}
			if got, err := os.ReadFile(path); err != nil || string(got) != material {
				t.Errorf("unarchived material was changed: bytes = %d, error = %v", len(got), err)
			}
			if got := read(t, cwd, "sibling"); got != "current sibling" {
				t.Errorf("rejected restore changed sibling to %q", got)
			}
		})
	}
}

func TestStoreRestoreRetainsAdmittedPreRestoreMaterial(t *testing.T) {
	s, cwd := newTestStore(t)
	write(t, cwd, "entry", "checkpoint")
	if err := s.Snapshot(t.Context(), "session", cwd, "boundary"); err != nil {
		t.Fatal(err)
	}
	write(t, cwd, "entry", "unsnapshotted current material")
	if err := s.Restore(t.Context(), "session", cwd, "boundary"); err != nil {
		t.Fatal(err)
	}
	if got := read(t, cwd, "entry"); got != "checkpoint" {
		t.Fatalf("restored file = %q", got)
	}
	archived, err := s.git(t.Context(), s.gitDir("session", cwd), cwd, "show", "HEAD@{1}:entry")
	if err != nil || archived != "unsnapshotted current material" {
		t.Fatalf("pre-restore archive = %q, error = %v", archived, err)
	}
}

func TestStoreRestoreStopsWhenPreRestoreArchiveFails(t *testing.T) {
	s, cwd := newTestStore(t)
	write(t, cwd, "entry", "checkpoint")
	if err := s.Snapshot(t.Context(), "session", cwd, "boundary"); err != nil {
		t.Fatal(err)
	}
	write(t, cwd, "entry", "current material")
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	script := fmt.Sprintf("#!/bin/sh\nif [ \"$1\" = commit ]; then exit 1; fi\nexec %q \"$@\"\n", realGit)
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	if err := s.Restore(t.Context(), "session", cwd, "boundary"); err == nil || errors.Is(err, ErrRestoreIncomplete) {
		t.Fatalf("restore = %v, want failure before checkout", err)
	}
	if got := read(t, cwd, "entry"); got != "current material" {
		t.Fatalf("uncaptured file changed to %q", got)
	}
}

func TestStoreRestoreProtectsUnarchivedAncestor(t *testing.T) {
	s, cwd := newTestStore(t)
	dir := filepath.Join(cwd, "entry")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, dir, "child", "checkpoint child")
	if err := s.Snapshot(t.Context(), "session", cwd, "boundary"); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	material := strings.Repeat("x", maxCheckpointFileSize+1)
	write(t, cwd, "entry", material)
	if err := s.Restore(t.Context(), "session", cwd, "boundary"); !errors.Is(err, ErrConflict) {
		t.Fatalf("restore = %v, want ancestor conflict", err)
	}
	if got := read(t, cwd, "entry"); got != material {
		t.Fatal("unarchived ancestor was overwritten")
	}
}

func TestStoreRestoreArchivesAndRestoresFileDirectoryTransitions(t *testing.T) {
	s, cwd := newTestStore(t)
	write(t, cwd, "entry", "checkpoint file")
	if err := s.Snapshot(t.Context(), "session", cwd, "file"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(cwd, "entry")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, path, "child", "checkpoint child")
	if err := s.Snapshot(t.Context(), "session", cwd, "directory"); err != nil {
		t.Fatal(err)
	}
	if err := s.Restore(t.Context(), "session", cwd, "file"); err != nil {
		t.Fatal(err)
	}
	if got := read(t, cwd, "entry"); got != "checkpoint file" {
		t.Fatalf("restored file = %q", got)
	}
	for range 2 {
		if err := s.Restore(t.Context(), "session", cwd, "directory"); err != nil {
			t.Fatal(err)
		}
		if got := read(t, path, "child"); got != "checkpoint child" {
			t.Fatalf("restored child = %q", got)
		}
	}
}

func TestStoreRestoreLiteralPathNeverAdmitsIgnoredGlobMatch(t *testing.T) {
	s, cwd := newTestStore(t)
	write(t, cwd, "literal*.txt", "checkpoint")
	write(t, cwd, ".gitignore", "literal-private.txt\n")
	if err := s.Snapshot(t.Context(), "session", cwd, "boundary"); err != nil {
		t.Fatal(err)
	}
	write(t, cwd, "literal*.txt", "current")
	write(t, cwd, "literal-private.txt", "unarchived")
	if err := s.Restore(t.Context(), "session", cwd, "boundary"); err != nil {
		t.Fatal(err)
	}
	if got := read(t, cwd, "literal*.txt"); got != "checkpoint" {
		t.Fatalf("literal path = %q", got)
	}
	if got := read(t, cwd, "literal-private.txt"); got != "unarchived" {
		t.Fatalf("ignored glob match = %q", got)
	}
}

func TestStoreRestoreProtectsIgnoredBlockerCreatedAfterPreflight(t *testing.T) {
	s, cwd := newTestStore(t)
	write(t, cwd, "entry", "checkpoint file")
	write(t, cwd, ".gitignore", "entry/\n")
	if err := s.Snapshot(t.Context(), "session", cwd, "boundary"); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(cwd, "entry")); err != nil {
		t.Fatal(err)
	}
	if err := s.Snapshot(t.Context(), "session", cwd, "later"); err != nil {
		t.Fatal(err)
	}
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	script := fmt.Sprintf("#!/bin/sh\nif [ \"$1\" = checkout ]; then\n mkdir -- \"$GIT_WORK_TREE/entry\" || exit 1\n printf 'external material' > \"$GIT_WORK_TREE/entry/late\" || exit 1\nfi\nexec %q \"$@\"\n", realGit)
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	if err := s.Restore(t.Context(), "session", cwd, "boundary"); !errors.Is(err, ErrRestoreIncomplete) || errors.Is(err, ErrConflict) {
		t.Fatalf("restore = %v, want conservative checkout failure", err)
	}
	if got := read(t, filepath.Join(cwd, "entry"), "late"); got != "external material" {
		t.Fatalf("late external material = %q", got)
	}
}
