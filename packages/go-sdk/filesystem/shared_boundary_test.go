package filesystem

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestOpenExpectedRejectsAnInPlaceChangeBeforeOpening pins the unified open-time
// contract: an admission requires the opened descriptor to match the caller's
// earlier observation by SameVersion, so a same-inode in-place rewrite between
// stat and open is rejected here rather than silently admitted.
func TestOpenExpectedRejectsAnInPlaceChangeBeforeOpening(t *testing.T) {
	path := filepath.Join(t.TempDir(), "input")
	if err := os.WriteFile(path, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	expected, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	// Rewrite in place: same inode, different size and modification time.
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("hello world"); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := OpenFileExpected(path, expected, 0); !errors.Is(err, ErrChanged) {
		t.Fatalf("OpenFileExpected after in-place change = %v, want ErrChanged", err)
	}
}

// TestOpenReportsOversizeAsTypedSizeLimitError pins the typed size error shared
// by both consumers: it carries the observed size and bound while remaining
// ErrFileTooLarge for callers that only branch on the category.
func TestOpenReportsOversizeAsTypedSizeLimitError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "input")
	if err := os.WriteFile(path, []byte("12345678"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _, err := OpenFile(path, 6)
	if !errors.Is(err, ErrFileTooLarge) {
		t.Fatalf("Open oversize error = %v, want ErrFileTooLarge", err)
	}
	var limit FileSizeLimitError
	if !errors.As(err, &limit) {
		t.Fatalf("Open oversize error = %v, want SizeLimitError", err)
	}
	if limit.Size != 8 || limit.Limit != 6 {
		t.Fatalf("SizeLimitError = %+v, want {Size:8 Limit:6}", limit)
	}
}
