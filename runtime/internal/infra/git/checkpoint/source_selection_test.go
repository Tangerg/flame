package checkpoint

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestStoreSourceSelectionHonorsWorkspaceAndGitOwnership(t *testing.T) {
	for _, nested := range []bool{false, true} {
		for _, split := range []bool{false, true} {
			t.Run(map[bool]string{false: "root", true: "nested"}[nested]+map[bool]string{false: "/ordinary", true: "/split"}[split], func(t *testing.T) {
				s, repo := newTestStore(t)
				gitCmd(t, repo, "init", "-q")
				cwd := repo
				if nested {
					cwd = filepath.Join(repo, "sub", "workspace")
				}
				if err := os.MkdirAll(filepath.Join(cwd, "build"), 0o755); err != nil {
					t.Fatal(err)
				}
				write(t, repo, ".gitignore", "secret.txt\n")
				write(t, repo, ".git/info/exclude", "private.txt\n")
				write(t, cwd, "build/owned.txt", "original")
				write(t, cwd, "build/generated.txt", "generated")
				write(t, cwd, "secret.txt", "ignored")
				write(t, cwd, "private.txt", "private")
				relative, err := filepath.Rel(repo, filepath.Join(cwd, "build", "owned.txt"))
				if err != nil {
					t.Fatal(err)
				}
				gitCmd(t, repo, "add", ".gitignore", relative)
				if split {
					gitCmd(t, repo, "update-index", "--split-index")
				}
				index, err := os.ReadFile(filepath.Join(repo, ".git", "index"))
				if err != nil {
					t.Fatal(err)
				}
				if err := s.Snapshot(t.Context(), "session", cwd, "before"); err != nil {
					t.Fatal(err)
				}
				output, err := s.gitOutput(t.Context(), s.gitDir("session", cwd), cwd, "ls-tree", "-r", "-z", "--name-only", tagFor("before"))
				if err != nil {
					t.Fatal(err)
				}
				paths, err := restorePaths(output)
				want := []string{"build/owned.txt"}
				if !nested {
					want = append([]string{".gitignore"}, want...)
				}
				if err != nil || !slices.Equal(paths, want) {
					t.Fatalf("captured paths = %q, %v; want %q", paths, err, want)
				}
				write(t, cwd, "build/owned.txt", "modified")
				write(t, cwd, "secret.txt", "current ignored")
				write(t, cwd, "private.txt", "current private")
				if err := s.Snapshot(t.Context(), "session", cwd, "after"); err != nil {
					t.Fatal(err)
				}
				if err := s.Restore(t.Context(), "session", cwd, "before"); err != nil {
					t.Fatal(err)
				}
				for path, want := range map[string]string{"build/owned.txt": "original", "secret.txt": "current ignored", "private.txt": "current private"} {
					if got := read(t, cwd, path); got != want {
						t.Errorf("%s = %q, want %q", path, got, want)
					}
				}
				if current, err := os.ReadFile(filepath.Join(repo, ".git", "index")); err != nil || !bytes.Equal(index, current) {
					t.Fatalf("source index changed: %v", err)
				}
			})
		}
	}
}

func TestStoreSourceSelectionReleasesNewlyIgnoredMaterial(t *testing.T) {
	s, cwd := newTestStore(t)
	gitCmd(t, cwd, "init", "-q")
	write(t, cwd, "entry", "original")
	if err := s.Snapshot(t.Context(), "session", cwd, "before"); err != nil {
		t.Fatal(err)
	}
	write(t, cwd, ".git/info/exclude", "entry\n")
	write(t, cwd, "entry", "private")
	if err := s.Snapshot(t.Context(), "session", cwd, "after"); err != nil {
		t.Fatal(err)
	}
	if err := s.Restore(t.Context(), "session", cwd, "before"); !errors.Is(err, ErrConflict) {
		t.Fatalf("restore = %v, want ignored-material conflict", err)
	}
	if got := read(t, cwd, "entry"); got != "private" {
		t.Fatalf("ignored material changed to %q", got)
	}
}

func TestStoreSourceSelectionUsesGlobalGitExcludes(t *testing.T) {
	s, cwd := newTestStore(t)
	gitCmd(t, cwd, "init", "-q")
	home := t.TempDir()
	write(t, home, "excludes", "private.txt\n")
	write(t, home, ".gitconfig", fmt.Sprintf("[core]\nexcludesFile = %q\n", filepath.Join(home, "excludes")))
	t.Setenv("HOME", home)
	write(t, cwd, "owned.txt", "original")
	write(t, cwd, "private.txt", "private")
	if err := s.Snapshot(t.Context(), "session", cwd, "before"); err != nil {
		t.Fatal(err)
	}
	write(t, cwd, "owned.txt", "modified")
	write(t, cwd, "private.txt", "current private")
	if err := s.Restore(t.Context(), "session", cwd, "before"); err != nil {
		t.Fatal(err)
	}
	if got := read(t, cwd, "owned.txt"); got != "original" {
		t.Fatalf("owned material = %q", got)
	}
	if got := read(t, cwd, "private.txt"); got != "current private" {
		t.Fatalf("globally ignored material = %q", got)
	}
}

func TestStoreRefusesFormerSnapshotOwnership(t *testing.T) {
	s, cwd := newTestStore(t)
	write(t, cwd, "entry", "original")
	if err := s.Snapshot(t.Context(), "session", cwd, "before"); err != nil {
		t.Fatal(err)
	}
	directory := s.gitDir("session", cwd)
	if err := os.Rename(filepath.Join(directory, workspaceIdentityFile), filepath.Join(directory, "flame-workspace")); err != nil {
		t.Fatal(err)
	}
	if err := s.Restore(t.Context(), "session", cwd, "before"); err == nil {
		t.Fatal("restored a repository without current source-selection ownership")
	}
	if got := read(t, cwd, "entry"); got != "original" {
		t.Fatalf("refused restore changed material to %q", got)
	}
}
