package fileinput

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenAdmitsOnlyBoundedRegularFiles(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "input")
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
	} else {
		var limit SizeLimitError
		if !errors.As(err, &limit) || limit.Size != 7 || limit.Limit != 6 {
			t.Fatalf("Open oversized detail = %+v, want size 7 limit 6", limit)
		}
	}
	if _, _, err := Open(root, 0); !errors.Is(err, ErrNotRegular) {
		t.Fatalf("Open directory error = %v, want ErrNotRegular", err)
	}
}

func TestOpenExpectedRejectsReplacement(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "input")
	if err := os.WriteFile(path, []byte("first"), 0o600); err != nil {
		t.Fatal(err)
	}
	expected, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	replacement := filepath.Join(root, "replacement")
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

func TestOpenDirectoryRejectsFiles(t *testing.T) {
	root := t.TempDir()
	directory, _, err := OpenDirectory(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := directory.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "file")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := OpenDirectory(path); !errors.Is(err, ErrNotDirectory) {
		t.Fatalf("OpenDirectory file error = %v, want ErrNotDirectory", err)
	}
}
