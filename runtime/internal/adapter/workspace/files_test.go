package workspace

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"

	workspaceapp "github.com/Tangerg/flame/runtime/internal/application/workspace"
)

// buildTree lays out a small non-git tree under t.TempDir() for the walk path
// (t.TempDir is outside any repo, so ListFiles takes the filesystem fallback).
func buildTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, f := range []string{
		"a.txt",
		"sub/b.go",
		"sub/c.go",
		"node_modules/dep/x.js", // backstop-excluded
		".git/HEAD",             // always excluded
	} {
		p := filepath.Join(root, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func paths(entries []workspaceapp.FileEntry) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.Path
	}
	return out
}

func TestListFiles_RecursiveSkipsBackstop(t *testing.T) {
	root := buildTree(t)
	got, err := ListFiles(context.Background(), root, workspaceapp.FileListOptions{Recursive: true})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"a.txt", "sub/b.go", "sub/c.go"}
	slices.Sort(want)
	gotP := paths(got)
	slices.Sort(gotP)
	if !slices.Equal(gotP, want) {
		t.Fatalf("recursive = %v, want %v (node_modules/.git must be excluded)", gotP, want)
	}
}

func TestListFiles_IncludeIgnoredSurfacesBackstop(t *testing.T) {
	root := buildTree(t)
	got, err := ListFiles(context.Background(), root, workspaceapp.FileListOptions{Recursive: true, IncludeIgnored: true})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(paths(got), "node_modules/dep/x.js") {
		t.Fatalf("includeIgnored should surface node_modules, got %v", paths(got))
	}
	if slices.Contains(paths(got), ".git/HEAD") {
		t.Fatal(".git must stay excluded even with includeIgnored")
	}
}

func TestListFiles_OneLevelDirsThenFiles(t *testing.T) {
	root := buildTree(t)
	got, err := ListFiles(context.Background(), root, workspaceapp.FileListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	// Root level: the `sub` dir (dirs sort first) then the `a.txt` file.
	if len(got) != 2 || got[0].Kind != workspaceapp.FileEntryDir || got[0].Name != "sub" {
		t.Fatalf("level[0] = %+v, want dir sub", got)
	}
	if got[1].Kind != workspaceapp.FileEntryFile || got[1].Name != "a.txt" {
		t.Fatalf("level[1] = %+v, want file a.txt", got[1])
	}
}

func TestListFiles_OneLevelIncludesEmptyDirectoryWithoutDescending(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := ListFiles(context.Background(), root, workspaceapp.FileListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"empty", "a.txt"}; !slices.Equal(paths(got), want) {
		t.Fatalf("paths = %v, want %v", paths(got), want)
	}
}

func TestLevelFilesystemEntriesRejectsEscapingDirectoryReplacement(t *testing.T) {
	root := t.TempDir()
	selected := filepath.Join(root, "selected")
	if err := os.Mkdir(selected, 0o755); err != nil {
		t.Fatal(err)
	}
	scope, err := resolveListDirectory(root, "selected")
	if err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(selected); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, selected); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	rootHandle, err := os.OpenRoot(scope.root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rootHandle.Close() })
	scope.handle = rootHandle
	if entries, err := levelFilesystemEntries(t.Context(), scope, true); err == nil {
		t.Fatalf("replaced directory entries = %+v, want confined-open error", entries)
	}
	if files, err := walkFiles(t.Context(), scope, true); err == nil {
		t.Fatalf("replaced directory walk = %v, want confined-walk error", files)
	}
}

func TestListFiles_HidesGitControlFileAndBoundsOneLevelReads(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{".git", "a.txt", "b.txt", "c.txt"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	for _, options := range []workspaceapp.FileListOptions{
		{IncludeIgnored: true},
		{Recursive: true, IncludeIgnored: true},
	} {
		got, err := ListFiles(context.Background(), root, options)
		if err != nil {
			t.Fatal(err)
		}
		if slices.Contains(paths(got), ".git") {
			t.Fatalf("ListFiles(%+v) exposed the Git control file: %v", options, paths(got))
		}
	}

	rootHandle, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rootHandle.Close() })
	if _, err := readDirectoryEntries(rootHandle, "", 3); !errors.Is(err, ErrListingTooLarge) {
		t.Fatalf("readDirectoryEntries() error = %v, want ErrListingTooLarge", err)
	}
}

func TestFileBrowserRejectsMissingAndNonDirectoryListPaths(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "file.txt"), []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, selected := range []string{"missing", "file.txt"} {
		_, err := (FileBrowser{}).List(t.Context(), root, workspaceapp.FileListOptions{Path: selected})
		if !errors.Is(err, workspaceapp.ErrInvalidFileListPath) {
			t.Errorf("List(%q) error = %v, want ErrInvalidFileListPath", selected, err)
		}
	}
}

func TestListFiles_ScopedToSubdir(t *testing.T) {
	root := buildTree(t)
	got, err := ListFiles(context.Background(), root, workspaceapp.FileListOptions{Path: "sub"})
	if err != nil {
		t.Fatal(err)
	}
	gotP := paths(got)
	slices.Sort(gotP)
	if !slices.Equal(gotP, []string{"sub/b.go", "sub/c.go"}) {
		t.Fatalf("sub level = %v", gotP)
	}
}

func TestListFilesProjectsSelectedDirectorySymlink(t *testing.T) {
	for _, repository := range []bool{false, true} {
		t.Run(map[bool]string{false: "filesystem", true: "git"}[repository], func(t *testing.T) {
			root := t.TempDir()
			if repository {
				if _, err := exec.LookPath("git"); err != nil {
					t.Skip("git is unavailable")
				}
				if output, err := exec.CommandContext(t.Context(), "git", "-C", root, "init", "-q").CombinedOutput(); err != nil {
					t.Fatalf("git init: %v: %s", err, output)
				}
			}
			if err := os.Mkdir(filepath.Join(root, "real"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "real", "visible.txt"), []byte("visible"), 0o644); err != nil {
				t.Fatal(err)
			}
			if repository {
				if err := os.WriteFile(filepath.Join(root, "real", "ignored.txt"), []byte("ignored"), 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("real/ignored.txt\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.Symlink("real", filepath.Join(root, "alias")); err != nil {
				t.Skipf("symlink unsupported: %v", err)
			}

			got, err := ListFiles(t.Context(), root, workspaceapp.FileListOptions{Path: "alias", Recursive: true})
			if err != nil {
				t.Fatal(err)
			}
			if gotPaths := paths(got); !slices.Equal(gotPaths, []string{"alias/visible.txt"}) {
				t.Fatalf("selected symlink paths = %v, want [alias/visible.txt]", gotPaths)
			}
		})
	}
}

func TestListFiles_GlobFilters(t *testing.T) {
	root := buildTree(t)
	got, err := ListFiles(context.Background(), root, workspaceapp.FileListOptions{Glob: "**/*.go"})
	if err != nil {
		t.Fatal(err)
	}
	gotP := paths(got)
	slices.Sort(gotP)
	if !slices.Equal(gotP, []string{"sub/b.go", "sub/c.go"}) {
		t.Fatalf("glob **/*.go = %v", gotP)
	}
}

func TestListFilesGlobUsesDoublestarRelativeToSelectedPath(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{
		"project/src/root.ts",
		"project/src/nested/deep.ts",
		"project/src/nested/deep.go",
		"project/other.ts",
	} {
		filename := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filename, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := ListFiles(context.Background(), root, workspaceapp.FileListOptions{
		Path: "project", Glob: "src/**/*.ts",
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotPaths := paths(got); !slices.Equal(gotPaths, []string{
		"project/src/nested/deep.ts",
		"project/src/root.ts",
	}) {
		t.Fatalf("scoped doublestar paths = %v", gotPaths)
	}
}

func TestListFilesRejectsInvalidGlob(t *testing.T) {
	t.Parallel()

	_, err := ListFiles(context.Background(), t.TempDir(), workspaceapp.FileListOptions{Glob: "["})
	if !errors.Is(err, ErrInvalidGlob) {
		t.Fatalf("ListFiles() error = %v, want ErrInvalidGlob", err)
	}
}

func TestListFilesInspectsMetadataAndSymlinks(t *testing.T) {
	root := buildTree(t)
	if err := os.Symlink("a.txt", filepath.Join(root, "a-link")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	got, err := ListFiles(context.Background(), root, workspaceapp.FileListOptions{Recursive: true})
	if err != nil {
		t.Fatal(err)
	}
	var file, link workspaceapp.FileEntry
	for _, entry := range got {
		switch entry.Path {
		case "a.txt":
			file = entry
		case "a-link":
			link = entry
		}
	}
	if file.Kind != workspaceapp.FileEntryFile || file.SizeBytes != 1 || file.ModifiedAt.IsZero() {
		t.Fatalf("file metadata = %+v", file)
	}
	if link.Kind != workspaceapp.FileEntrySymlink {
		t.Fatalf("symlink metadata = %+v", link)
	}
}

func TestListFilesHonorsCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := ListFiles(ctx, t.TempDir(), workspaceapp.FileListOptions{Recursive: true, IncludeIgnored: true})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ListFiles() error = %v, want context.Canceled", err)
	}
}
