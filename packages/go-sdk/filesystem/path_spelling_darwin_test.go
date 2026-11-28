package filesystem

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestResolvePreservesDirectorySpellingForMissingDescendants(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(root, "Checkout")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, "CHECKOUT")
	if _, err := os.Stat(alias); errors.Is(err, os.ErrNotExist) {
		t.Skip("the working volume is case-sensitive")
	} else if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(directory, 0o100); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(directory, 0o700); err != nil {
			t.Error(err)
		}
	})
	for _, suffix := range []string{".", "missing/child"} {
		got, err := ResolvePath("", filepath.Join(alias, suffix))
		if want := filepath.Join(directory, suffix); err != nil || got != want {
			t.Errorf("resolve %q = %q, %v; want %q", suffix, got, err, want)
		}
	}
}
