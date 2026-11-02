//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd

package distribution

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestArchiveRefusesFIFOWithoutWaitingForWriter(t *testing.T) {
	s := storeFixture(t, nil)
	path := filepath.Join(t.TempDir(), "pkg.zip")
	if err := syscall.Mkfifo(path, 0600); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := s.InstallLocalArchiveHeld(context.Background(), path); done <- err }()
	select {
	case err := <-done:
		if !errors.Is(err, ErrCapacity) {
			t.Fatalf("FIFO admission: %v", err)
		}
	case <-time.After(time.Second):
		f, err := os.OpenFile(path, os.O_WRONLY|syscall.O_NONBLOCK, 0)
		if err == nil {
			_ = f.Close()
		}
		t.Fatal("archive admission blocked waiting for a FIFO writer")
	}
}
