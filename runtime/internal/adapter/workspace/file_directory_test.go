package workspace

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	workspaceapp "github.com/Tangerg/flame/runtime/internal/application/workspace"
)

func TestDirectoryPagesCoverOneLevel(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	for index := range 37 {
		for _, directory := range []string{root, filepath.Join(root, "nested")} {
			if err := os.WriteFile(filepath.Join(directory, fmt.Sprintf("entry-%02d", index)), nil, 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
	plan := workspaceapp.DirectoryListPlan{Limit: 7}
	seen := make(map[string]bool)
	scanned := 0
	for {
		page, err := (FileBrowser{}).ListDirectory(t.Context(), root, plan)
		if err != nil {
			t.Fatal(err)
		}
		scanned += page.ScannedEntries
		for _, entry := range page.Entries {
			if seen[entry.Path] {
				t.Fatalf("duplicate entry %s", entry.Path)
			}
			seen[entry.Path] = true
		}
		if page.Complete {
			break
		}
		if page.NextOffset <= plan.Offset {
			t.Fatal("cursor did not advance")
		}
		plan.Offset, plan.Revision = page.NextOffset, page.Revision
	}
	if len(seen) != 38 || scanned != 38 {
		t.Fatalf("covered %d entries, scanned %d; want the 38 direct children", len(seen), scanned)
	}
}

func TestDirectoryContinuationRefusesChangedSource(t *testing.T) {
	root := t.TempDir()
	for index := range 3 {
		if err := os.WriteFile(filepath.Join(root, fmt.Sprint(index)), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	page, err := (FileBrowser{}).ListDirectory(t.Context(), root, workspaceapp.DirectoryListPlan{Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	changed := time.Now().Add(time.Second)
	if err := os.Chtimes(root, changed, changed); err != nil {
		t.Fatal(err)
	}
	_, err = (FileBrowser{}).ListDirectory(t.Context(), root, workspaceapp.DirectoryListPlan{Limit: 1, Offset: page.NextOffset, Revision: page.Revision})
	if !errors.Is(err, workspaceapp.ErrPageCursor) {
		t.Fatalf("changed source = %v", err)
	}
}

func TestDirectoryBudgetAndTrackedIgnoreExceptions(t *testing.T) {
	root := t.TempDir()
	gitCommand(t, root, "init", "-q")
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("ignored-*\nkept/\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "kept"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "kept", "tracked"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	gitCommand(t, root, "add", "-f", "kept/tracked")
	for index := range 300 {
		if err := os.WriteFile(filepath.Join(root, fmt.Sprintf("ignored-%03d", index)), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	plan := workspaceapp.DirectoryListPlan{Limit: 10}
	exhausted, kept := false, false
	for {
		page, err := (FileBrowser{}).ListDirectory(t.Context(), root, plan)
		if err != nil {
			t.Fatal(err)
		}
		exhausted = exhausted || page.BudgetExhausted
		for _, entry := range page.Entries {
			if entry.Path == "kept" {
				kept = true
			}
			if entry.Name == ".git" {
				t.Fatal("listed git control directory")
			}
		}
		if page.Complete {
			break
		}
		plan.Offset, plan.Revision = page.NextOffset, page.Revision
	}
	if !exhausted || !kept {
		t.Fatalf("budget exhausted=%v, tracked ignored directory=%v", exhausted, kept)
	}
}
