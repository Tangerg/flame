package main

import (
	"os"
	"path/filepath"
	"testing"
)

func writeHooks(t *testing.T, dir, body string) {
	t.Helper()
	flame := filepath.Join(dir, ".flame")
	if err := os.MkdirAll(flame, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(flame, "hooks.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
