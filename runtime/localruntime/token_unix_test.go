//go:build unix

package localruntime

import (
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestReadTokenFileRejectsFIFOReplacementWithoutBlocking(t *testing.T) {
	path := filepath.Join(t.TempDir(), "local-token")
	value := base64.RawURLEncoding.EncodeToString(make([]byte, rawTokenBytes))
	if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
		t.Fatal(err)
	}
	expected, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readTokenFile(path, expected); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("readTokenFile() error = %v, want ErrInvalidToken", err)
	}
}
