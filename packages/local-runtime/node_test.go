package localruntime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
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

func TestNodeVerificationRejectsChangedOrRedirectedBytes(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "node")
	content := []byte("qualified fixture")
	hash := sha256.Sum256(content)
	selected := Node{command: path, distribution: directory, expected: hex.EncodeToString(hash[:])}
	if err := os.WriteFile(path, content, 0700); err != nil {
		t.Fatal(err)
	}
	if err := selected.Verify(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("changed"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := selected.Verify(t.Context()); err == nil {
		t.Fatal("changed executable admitted")
	}
	if err := os.Rename(path, path+"-other"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(path+"-other", path); err != nil {
		t.Fatal(err)
	}
	if err := selected.Verify(t.Context()); err == nil {
		t.Fatal("redirected executable admitted")
	}
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	if err := (Node{}).Verify(cancelled); err == nil {
		t.Fatal("invalid zero selection admitted")
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
