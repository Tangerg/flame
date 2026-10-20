package localruntime

import (
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestReadOpenedTokenFileRejectsPathReplacement(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "local-token")
	first := base64.RawURLEncoding.EncodeToString(make([]byte, rawTokenBytes))
	if err := os.WriteFile(path, []byte(first), 0o600); err != nil {
		t.Fatal(err)
	}
	pathInfo, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	file, err := openReadOnlyPath(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()

	replacement := filepath.Join(root, "replacement")
	secondBytes := make([]byte, rawTokenBytes)
	secondBytes[0] = 1
	second := base64.RawURLEncoding.EncodeToString(secondBytes)
	if err := os.WriteFile(replacement, []byte(second), 0o600); err != nil {
		t.Fatal(err)
	}
	displaced := filepath.Join(root, "displaced")
	if err := os.Rename(path, displaced); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, path); err != nil {
		t.Fatal(err)
	}

	if _, err := readOpenedTokenFile(path, file, pathInfo); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("readOpenedTokenFile() error = %v, want ErrInvalidToken", err)
	}
}
