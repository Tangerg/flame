package toolset

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFormatPathIgnoresDeletedFile(t *testing.T) {
	if err := formatTestPath(t, filepath.Join(t.TempDir(), "deleted.go")); err != nil {
		t.Fatalf("format deleted file: %v", err)
	}
}

func TestFormatPathDoesNotReplaceSymbolicLink(t *testing.T) {
	directory := t.TempDir()
	target := filepath.Join(directory, "target.json")
	before := []byte(`{"value":1}`)
	if err := os.WriteFile(target, before, 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(directory, "linked.json")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if err := formatTestPath(t, link); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("formatted link = (%v, %v), want symbolic link", info, err)
	}
	if content, err := os.ReadFile(target); err != nil || !bytes.Equal(content, before) {
		t.Fatalf("target = %q, %v; want unchanged", content, err)
	}
}

func TestApplyFormattedFileRejectsReplacedSource(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "data.json")
	if err := os.WriteFile(path, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	root := mustRoot(t, directory)
	executor := mustLocalExecutor(t, directory)
	source, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	replacement := filepath.Join(directory, "replacement")
	if err := os.WriteFile(replacement, []byte("replacement"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, path); err != nil {
		t.Fatal(err)
	}
	if err := applyFormattedFile(t.Context(), root, executor, "data.json", []byte("formatted"), autoFormatSource{content: "original", info: source}); err == nil || !strings.Contains(err.Error(), "changed while formatting") {
		t.Fatalf("applyFormattedFile error = %v, want changed source", err)
	}
	if content, err := os.ReadFile(path); err != nil || string(content) != "replacement" {
		t.Fatalf("replacement = %q, %v; want preserved", content, err)
	}
}

func TestApplyFormattedFileDoesNotRecreateDeletedDirectory(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "removed")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "data.json")
	if err := os.WriteFile(path, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	root := mustRoot(t, directory)
	executor := mustLocalExecutor(t, directory)
	source, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(directory); err != nil {
		t.Fatal(err)
	}
	if err := applyFormattedFile(t.Context(), root, executor, "data.json", []byte("formatted"), autoFormatSource{content: "original", info: source}); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("formatting a removed source error = %v, want filesystem cause", err)
	}
	if _, err := os.Lstat(directory); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("removed directory was recreated: %v", err)
	}
}

func TestFormatPathSurfacesUnexpectedStatFailure(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(parent, []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := formatPath(t.Context(), &testCodeIntelligence{}, mustRoot(t, filepath.Dir(parent)), mustLocalExecutor(t, filepath.Dir(parent)), "file/child.go")
	if err == nil || !strings.Contains(err.Error(), "inspect before formatting") {
		t.Fatalf("format error = %v, want stat context", err)
	}
}

func TestFormatPathRefusesOversizedSupportedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data.json")
	content := `{"value":"` + strings.Repeat("x", (8<<20)+1) + `"}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	err := formatTestPath(t, path)
	if err == nil || !strings.Contains(err.Error(), "2 MiB") {
		t.Fatalf("format oversized JSON error = %v, want explicit 8 MiB refusal", err)
	}
}

func formatTestPath(t *testing.T, path string) error {
	t.Helper()
	directory := filepath.Dir(path)
	return formatPath(t.Context(), &testCodeIntelligence{}, mustRoot(t, directory), mustLocalExecutor(t, directory), filepath.Base(path))
}
