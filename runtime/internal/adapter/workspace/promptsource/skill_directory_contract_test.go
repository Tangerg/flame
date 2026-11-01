package promptsource

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	workspaceapp "github.com/Tangerg/flame/runtime/internal/application/workspace"
	sdk "github.com/Tangerg/scope/skills"
)

func TestSkillDirectoryCanAppearAfterSourceCreation(t *testing.T) {
	boundary := t.TempDir()
	root := filepath.Join(boundary, "skills")
	source, err := NewSkillSource(root, boundary)
	if err != nil {
		t.Fatal(err)
	}
	if found, err := source.List(t.Context()); err != nil || len(found) != 0 {
		t.Fatalf("absent directory: %v, %v", found, err)
	}
	if _, err := source.Lookup(t.Context(), "review"); !errors.Is(err, sdk.ErrSkillNotFound) {
		t.Fatalf("absent skill: %v", err)
	}
	writeRuntimeSkill(t, root, "review", "Read the evidence.")
	if got, err := source.Lookup(t.Context(), "review"); err != nil || got.Name != "review" {
		t.Fatalf("created directory: %+v, %v", got, err)
	}
}

func TestSkillDirectoryRedirectDoesNotReplaceReadAuthority(t *testing.T) {
	boundary := t.TempDir()
	root := filepath.Join(boundary, "skills")
	outside := t.TempDir()
	const name, resource = "review", "references/note.txt"
	for _, library := range []string{root, outside} {
		writeRuntimeSkill(t, library, name, "Read the evidence.")
		path := filepath.Join(library, name, filepath.FromSlash(resource))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(library), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	source, err := NewSkillSource(root, boundary)
	if err != nil {
		t.Fatal(err)
	}
	file, err := source.OpenResource(t.Context(), name, resource)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := file.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := os.Rename(root, filepath.Join(boundary, "old")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, root); err != nil {
		t.Skipf("directory symlink unavailable: %v", err)
	}
	data, err := io.ReadAll(file)
	if err != nil || string(data) != root {
		t.Fatalf("acquired resource changed: %q, %v", data, err)
	}
	if _, err := source.List(t.Context()); !errors.Is(err, workspaceapp.ErrPathOutsideRoot) {
		t.Errorf("list followed replacement: %v", err)
	}
	if _, err := source.Lookup(t.Context(), name); !errors.Is(err, workspaceapp.ErrPathOutsideRoot) {
		t.Errorf("lookup followed replacement: %v", err)
	}
	if _, err := source.Load(t.Context(), name); !errors.Is(err, workspaceapp.ErrPathOutsideRoot) {
		t.Errorf("load followed replacement: %v", err)
	}
	opened, err := source.OpenResource(t.Context(), name, resource)
	if opened != nil {
		_ = opened.Close()
		t.Error("granted new resource after redirection")
	}
	if !errors.Is(err, workspaceapp.ErrPathOutsideRoot) {
		t.Errorf("resource followed replacement: %v", err)
	}
}
