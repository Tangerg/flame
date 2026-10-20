package directories

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Tangerg/flame/runtime/workspace"

	"github.com/Tangerg/flame/runtime/internal/infra/filesystem/pathidentity"
)

type Resolver struct{}

// An unresolved physical identity cannot become a competing lexical identity.
// Relative values are rejected because they depend on the process cwd.
func Canonical(path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", workspace.ErrAbsolutePathRequired
	}
	return pathidentity.Resolve("", path)
}

func (Resolver) ResolveExistingDir(path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", workspace.ErrAbsolutePathRequired
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", workspace.ErrNotDirectory
	}
	return Canonical(path)
}

func (r Resolver) ResolveRoot(path string) (string, error) {
	canonical, err := r.ResolveExistingDir(path)
	if err != nil {
		return "", fmt.Errorf("%w: %s: %w", workspace.ErrCWDUnavailable, path, err)
	}
	return canonical, nil
}
