package fileinput

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenAndOpenAtAdmitOnlyBoundedRegularFiles(t *testing.T) {
	rootPath := t.TempDir()
	path := filepath.Join(rootPath, "input")
	if err := os.WriteFile(path, []byte("content"), 0o600); err != nil {
		t.Fatal(err)
	}
	file, info, err := Open(path, 7)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != 7 {
		t.Fatalf("opened size = %d, want 7", info.Size())
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Open(path, 6); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("Open oversized error = %v, want ErrTooLarge", err)
	}
	if _, _, err := Open(rootPath, 0); !errors.Is(err, ErrNotRegular) {
		t.Fatalf("Open directory error = %v, want ErrNotRegular", err)
	}

	root, err := os.OpenRoot(rootPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	file, _, err = OpenAt(root, "input", 7)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestOpenExpectedRejectsAReplacement(t *testing.T) {
	rootPath := t.TempDir()
	path := filepath.Join(rootPath, "input")
	if err := os.WriteFile(path, []byte("first"), 0o600); err != nil {
		t.Fatal(err)
	}
	expected, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	replacement := filepath.Join(rootPath, "replacement")
	if err := os.WriteFile(replacement, []byte("second"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, path); err != nil {
		t.Fatal(err)
	}
	if _, _, err := OpenExpected(path, expected, 0); !errors.Is(err, ErrChanged) {
		t.Fatalf("OpenExpected replacement error = %v, want ErrChanged", err)
	}
}

func TestSameVersionRequiresStableIdentityAndMetadata(t *testing.T) {
	rootPath := t.TempDir()
	path := filepath.Join(rootPath, "input")
	if err := os.WriteFile(path, []byte("first"), 0o600); err != nil {
		t.Fatal(err)
	}
	first, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	unchanged, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !SameVersion(first, unchanged) {
		t.Fatal("unchanged file versions differ")
	}
	if SameVersion(nil, unchanged) {
		t.Fatal("nil file version was accepted")
	}
	if err := os.WriteFile(path, []byte("changed size"), 0o600); err != nil {
		t.Fatal(err)
	}
	changed, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if SameVersion(first, changed) {
		t.Fatal("changed file version was accepted")
	}
}

func TestOpenDirectoryAdmitsOnlyTheExpectedDirectory(t *testing.T) {
	rootPath := t.TempDir()
	directory := filepath.Join(rootPath, "directory")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	opened, info, err := OpenDirectory(directory)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() {
		t.Fatalf("opened mode = %s, want directory", info.Mode())
	}
	if err := opened.Close(); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(rootPath, "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := OpenDirectory(file); !errors.Is(err, ErrNotDirectory) {
		t.Fatalf("OpenDirectory file error = %v, want ErrNotDirectory", err)
	}
}

func TestOpenDirectoryAtExpectedRejectsAReplacement(t *testing.T) {
	rootPath := t.TempDir()
	directory := filepath.Join(rootPath, "directory")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	expected, err := os.Stat(directory)
	if err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	replacement := filepath.Join(rootPath, "replacement")
	if err := os.Mkdir(replacement, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(directory); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, directory); err != nil {
		t.Fatal(err)
	}
	if _, _, err := OpenDirectoryAtExpected(root, "directory", expected); !errors.Is(err, ErrChanged) {
		t.Fatalf("OpenDirectoryAtExpected replacement error = %v, want ErrChanged", err)
	}
}
