package delivery

import (
	"testing"

	workspaceadapter "github.com/Tangerg/flame/runtime/internal/adapter/workspace"
)

func canonicalWorkspacePath(t *testing.T, path string) string {
	t.Helper()
	canonical, err := workspaceadapter.Canonical(path)
	if err != nil {
		t.Fatalf("canonical workspace path %q: %v", path, err)
	}
	return canonical
}
