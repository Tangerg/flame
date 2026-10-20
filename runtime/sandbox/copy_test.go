package sandbox

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/infra/filesystem/fileinput"
)

func TestCopyTreePreservesDirectoryAndSymlinkSemantics(t *testing.T) {
	source := t.TempDir()
	destination := t.TempDir()
	if err := os.Mkdir(filepath.Join(source, "nested"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "nested", "note.txt"), []byte("note"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("nested/note.txt", filepath.Join(source, "note-link")); err != nil {
		t.Fatal(err)
	}

	if err := copyTree(t.Context(), source, destination); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(destination, "note-link"))
	if err != nil || string(content) != "note" {
		t.Fatalf("copied symlink content = (%q, %v)", content, err)
	}
	linkInfo, err := os.Lstat(filepath.Join(destination, "note-link"))
	if err != nil || linkInfo.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("copied link info = (%v, %v), want symlink", linkInfo, err)
	}
	directoryInfo, err := os.Stat(filepath.Join(destination, "nested"))
	if err != nil || directoryInfo.Mode().Perm() != 0o750 {
		t.Fatalf("copied directory mode = (%v, %v), want 0750", directoryInfo, err)
	}
}

func TestCopyTreeRejectsEscapingSymlink(t *testing.T) {
	source := t.TempDir()
	if err := os.Symlink("../outside", filepath.Join(source, "escape")); err != nil {
		t.Fatal(err)
	}
	if err := copyTree(t.Context(), source, t.TempDir()); err == nil {
		t.Fatal("escaping source symlink was accepted")
	}
}

func TestCopyTreeRequiresIndependentAbsoluteRoots(t *testing.T) {
	source := t.TempDir()
	insideDestination := filepath.Join(source, "nested")
	if err := os.Mkdir(insideDestination, 0o700); err != nil {
		t.Fatal(err)
	}
	linkedParent := t.TempDir()
	linkedDestination := filepath.Join(linkedParent, "destination")
	if err := os.Symlink(source, linkedDestination); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name        string
		source      string
		destination string
	}{
		{name: "relative source", source: "relative", destination: t.TempDir()},
		{name: "relative destination", source: source, destination: "relative"},
		{name: "destination inside source", source: source, destination: insideDestination},
		{name: "destination resolves to source", source: source, destination: linkedDestination},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := copyTree(t.Context(), test.source, test.destination); err == nil {
				t.Fatal("unsafe workspace roots were accepted")
			}
		})
	}
}

func TestOpenCopyRootRejectsReplacedResolvedDirectory(t *testing.T) {
	parent := t.TempDir()
	name := filepath.Join(parent, "source")
	if err := os.Mkdir(name, 0o755); err != nil {
		t.Fatal(err)
	}
	resolved, err := resolveCopyRoot(name, "source")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(name, filepath.Join(parent, "retired")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(name, 0o755); err != nil {
		t.Fatal(err)
	}

	root, _, err := openCopyRoot(resolved, "source")
	if root != nil {
		_ = root.Close()
	}
	if err == nil || !strings.Contains(err.Error(), "changed while it was being opened") {
		t.Fatalf("open replaced root error = %v", err)
	}
}

func TestCopyTreeHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	err := copyTree(ctx, t.TempDir(), t.TempDir())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("copyTree error = %v, want context.Canceled", err)
	}
}

func TestTreeCopierCountsDirectoriesAgainstTheEntryLimit(t *testing.T) {
	source := t.TempDir()
	for _, name := range []string{"a/nested", "b"} {
		if err := os.MkdirAll(filepath.Join(source, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := copyTreeWithEntryLimit(t, source, 2); err == nil || !strings.Contains(err.Error(), "more than 2 entries") {
		t.Fatalf("two-entry copy error = %v", err)
	}
	destination, err := copyTreeWithEntryLimit(t, source, 3)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a", "a/nested", "b"} {
		if info, err := os.Stat(filepath.Join(destination, name)); err != nil || !info.IsDir() {
			t.Fatalf("copied directory %q = %v, %v", name, info, err)
		}
	}
}

func TestTreeCopierRejectsReplacedSourceDirectory(t *testing.T) {
	source := t.TempDir()
	destination := t.TempDir()
	original := filepath.Join(source, "nested")
	if err := os.Mkdir(original, 0o755); err != nil {
		t.Fatal(err)
	}
	expected, err := os.Stat(original)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(original, filepath.Join(source, "retired")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(original, 0o755); err != nil {
		t.Fatal(err)
	}

	sourceRoot, _, err := openResolvedCopyRoot(source, "source")
	if err != nil {
		t.Fatal(err)
	}
	destinationRoot, _, err := openResolvedCopyRoot(destination, "destination")
	if err != nil {
		_ = sourceRoot.Close()
		t.Fatal(err)
	}
	defer func() { _ = errors.Join(destinationRoot.Close(), sourceRoot.Close()) }()
	copier := treeCopier{
		source: sourceRoot, destination: destinationRoot,
		buffer: make([]byte, workspaceCopyBufferBytes), maxEntries: maxWorkspaceCopyEntries,
	}
	if err := copier.copyDirectory(t.Context(), "nested", expected); err == nil || !errors.Is(err, fileinput.ErrChanged) {
		t.Fatalf("copy replaced directory error = %v, want fileinput.ErrChanged", err)
	}
}

func TestTreeCopierRejectsReplacedSourceSymlink(t *testing.T) {
	source := t.TempDir()
	destination := t.TempDir()
	name := filepath.Join(source, "link")
	if err := os.Symlink("first", name); err != nil {
		t.Fatal(err)
	}
	expected, err := os.Lstat(name)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(name); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("second", name); err != nil {
		t.Fatal(err)
	}

	sourceRoot, _, err := openResolvedCopyRoot(source, "source")
	if err != nil {
		t.Fatal(err)
	}
	destinationRoot, _, err := openResolvedCopyRoot(destination, "destination")
	if err != nil {
		_ = sourceRoot.Close()
		t.Fatal(err)
	}
	defer func() { _ = errors.Join(destinationRoot.Close(), sourceRoot.Close()) }()
	copier := treeCopier{source: sourceRoot, destination: destinationRoot}
	if err := copier.copySymlink("link", "link", expected); err == nil || !errors.Is(err, fileinput.ErrChanged) {
		t.Fatalf("copy replaced symlink error = %v, want fileinput.ErrChanged", err)
	}
}

func copyTreeWithEntryLimit(t *testing.T, source string, limit int) (_ string, err error) {
	t.Helper()
	destination := t.TempDir()
	sourceRoot, sourceInfo, err := openResolvedCopyRoot(source, "source")
	if err != nil {
		return "", err
	}
	destinationRoot, _, err := openResolvedCopyRoot(destination, "destination")
	if err != nil {
		return "", errors.Join(err, sourceRoot.Close())
	}
	defer func() { err = errors.Join(err, destinationRoot.Close(), sourceRoot.Close()) }()
	copier := treeCopier{
		source: sourceRoot, destination: destinationRoot,
		buffer: make([]byte, workspaceCopyBufferBytes), maxEntries: limit,
	}
	if err := copier.copyDirectory(t.Context(), ".", sourceInfo); err != nil {
		return "", err
	}
	if err := copier.restoreDirectoryModes(t.Context()); err != nil {
		return "", err
	}
	return destination, nil
}

func openResolvedCopyRoot(name, role string) (*os.Root, os.FileInfo, error) {
	resolved, err := resolveCopyRoot(name, role)
	if err != nil {
		return nil, nil, err
	}
	return openCopyRoot(resolved, role)
}
