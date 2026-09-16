package persistence

import (
	"path/filepath"
	"testing"
)

func TestPingAnswersWhileOpenAndFailsOnceClosed(t *testing.T) {
	root := t.TempDir()
	bundle, err := Open(t.Context(), Config{
		DataDirectory: filepath.Join(root, "data"), DefaultWorkspacePath: root,
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := bundle.Ping(t.Context()); err != nil {
		t.Fatalf("Ping on an open bundle: %v", err)
	}
	if err := bundle.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := bundle.Ping(t.Context()); err == nil {
		t.Fatal("Ping reported healthy storage after the handle was released")
	}
}
