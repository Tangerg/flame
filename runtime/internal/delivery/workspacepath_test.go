package delivery

import (
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/storage/directories"
)

func canonicalWorkspacePath(t *testing.T, path string) string {
	t.Helper()
	canonical, err := directories.Canonical(path)
	if err != nil {
		t.Fatalf("canonical workspace path %q: %v", path, err)
	}
	return canonical
}
