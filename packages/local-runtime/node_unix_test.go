//go:build unix

package localruntime

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestVerifyRefusesFIFOWithoutAWriter(t *testing.T) {
	root := t.TempDir()
	n, err := NodeForExecutable(filepath.Join(root, "flame"))
	if err != nil {
		t.Skipf("no companion for host: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(n.Command()), 0700); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(n.Command(), 0600); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- n.Verify(t.Context()) }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("accepted pipe")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("pipe open blocked verification")
	}
}
