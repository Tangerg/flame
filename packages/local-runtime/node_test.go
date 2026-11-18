package localruntime

import (
	"path/filepath"
	"slices"
	"testing"
)

func TestNodeSelectionUsesOnlyHostDistribution(t *testing.T) {
	if _, err := NodeForExecutable("node"); err == nil {
		t.Fatal("relative host executable admitted")
	}
	executable := filepath.Join(t.TempDir(), "host with 空格", "flame")
	selected, err := NodeForExecutable(executable)
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(selected.Command()) || !filepath.IsLocal(mustRelative(t, filepath.Dir(executable), selected.Command())) {
		t.Fatal("companion escaped host distribution")
	}
	args := selected.Arguments("/leased entry/start.mjs")
	if !slices.Equal(args[len(args)-2:], []string{"--", "/leased entry/start.mjs"}) {
		t.Fatal("entry is not a distinct process argument")
	}
	args[0] = "--import=untrusted"
	if selected.Arguments("/other")[0] == args[0] {
		t.Fatal("caller changed launch policy")
	}
	if err := selected.Verify(t.Context()); err == nil {
		t.Fatal("missing companion admitted")
	}
}

func mustRelative(t *testing.T, root, path string) string {
	t.Helper()
	relative, err := filepath.Rel(root, path)
	if err != nil {
		t.Fatal(err)
	}
	return relative
}
