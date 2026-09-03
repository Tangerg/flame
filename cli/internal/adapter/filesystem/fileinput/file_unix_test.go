//go:build unix

package fileinput

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestOpenExpectedDoesNotBlockOnFIFOReplacement(t *testing.T) {
	path := filepath.Join(t.TempDir(), "input")
	if err := os.WriteFile(path, []byte("content"), 0o600); err != nil {
		t.Fatal(err)
	}
	expected, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := OpenExpected(path, expected, 0); !errors.Is(err, ErrChanged) {
		t.Fatalf("OpenExpected FIFO error = %v, want ErrChanged", err)
	}
}
