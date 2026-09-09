package project

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestRootFindsTheNearestMarkedAncestor(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, marker), 0o755); err != nil {
		t.Fatal(err)
	}
	leaf := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(leaf, 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := Root(leaf)
	if err != nil || got != root {
		t.Fatalf("Root(%q) = (%q, %v), want %q", leaf, got, err, root)
	}
	unmarked := t.TempDir()
	got, err = Root(filepath.Join(unmarked, ".") + string(filepath.Separator))
	if err != nil || got != unmarked {
		t.Fatalf("Root of an unmarked tree = (%q, %v), want the cleaned cwd %q", got, err, unmarked)
	}
}

// TestRootReportsADirectoryItCannotInspect covers the rule the three former
// copies disagreed on. Walking past an unreadable directory answers with some
// higher root, and the root is what a trust lookup is keyed by.
func TestRootReportsADirectoryItCannotInspect(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	blocked := filepath.Join(base, "blocked")
	leaf := filepath.Join(blocked, "leaf")
	if err := os.MkdirAll(leaf, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(blocked, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(blocked, 0o755) })
	if _, err := os.Stat(filepath.Join(leaf, marker)); err == nil || os.IsNotExist(err) {
		t.Skip("this filesystem or user can still inspect a 0o000 directory")
	}
	if _, err := Root(leaf); err == nil {
		t.Fatal("Root walked past a directory it could not inspect")
	}
}

func TestChainRunsRootToLeafInclusive(t *testing.T) {
	t.Parallel()

	root := filepath.Join("/tmp", "project")
	leaf := filepath.Join(root, "a", "b")
	want := []string{root, filepath.Join(root, "a"), leaf}
	if got := Chain(leaf, root); !reflect.DeepEqual(got, want) {
		t.Fatalf("Chain = %q, want %q", got, want)
	}
	if got := Chain(root, root); !reflect.DeepEqual(got, []string{root}) {
		t.Fatalf("Chain of a root = %q, want one element", got)
	}
}
