package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	workspaceapp "github.com/Tangerg/flame/runtime/internal/application/workspace"
)

func TestFileBrowserReadConfinesSymlinksToWorkspace(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "nested")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "inside.txt"), []byte("inside"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join("nested", "inside.txt"), filepath.Join(root, "inside-link")); err != nil {
		t.Fatal(err)
	}
	read, err := (FileBrowser{}).Read(t.Context(), root, workspaceapp.FileReadPlan{Path: "inside-link", MaxBytes: workspaceapp.DefaultFileReadBytes})
	if err != nil {
		t.Fatal(err)
	}
	if read.Content != "inside" {
		t.Fatalf("confined symlink content = %q, want inside", read.Content)
	}

	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "outside.txt"), []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "outside.txt"), filepath.Join(root, "outside-link")); err != nil {
		t.Fatal(err)
	}
	if _, err := (FileBrowser{}).Read(
		t.Context(), root, workspaceapp.FileReadPlan{Path: "outside-link", MaxBytes: workspaceapp.DefaultFileReadBytes},
	); !errors.Is(err, workspaceapp.ErrPathOutsideRoot) {
		t.Fatalf("escaping symlink error = %v, want ErrPathOutsideRoot", err)
	}
}
