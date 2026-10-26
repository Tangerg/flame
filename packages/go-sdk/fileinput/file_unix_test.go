//go:build unix

package fileinput

import (
	"errors"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestOpenDoesNotBlockOnFIFO(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pipe")
	if err := unix.Mkfifo(path, 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := openPath(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Open(path, 1); !errors.Is(err, ErrNotRegular) {
		t.Fatalf("Open FIFO error = %v, want ErrNotRegular", err)
	}
}
