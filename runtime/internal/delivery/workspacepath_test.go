package delivery

import (
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
)

func canonicalWorkspacePath(t *testing.T, path string) string {
	t.Helper()
	canonical, err := filesystem.CanonicalPath(path)
	if err != nil {
		t.Fatalf("canonical workspace path %q: %v", path, err)
	}
	return canonical
}
