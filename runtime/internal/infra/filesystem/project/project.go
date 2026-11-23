package project

import (
	"errors"
	"os"
	"path/filepath"
	"slices"

	"github.com/Tangerg/flame/runtime/internal/infra/git"
)

// An uninspectable directory is an error, not an absence: walking past the real
// root would key hook trust and the document cascade to a different one.
func Root(cwd string) (string, error) {
	current := filepath.Clean(cwd)
	for {
		switch _, err := os.Stat(filepath.Join(current, git.MetadataDirectory)); {
		case err == nil:
			return current, nil
		case !errors.Is(err, os.ErrNotExist):
			return "", err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return filepath.Clean(cwd), nil
		}
		current = parent
	}
}

func Chain(cwd, root string) []string {
	if cwd == root {
		return []string{cwd}
	}
	var chain []string
	current := cwd
	for current != root {
		chain = append(chain, current)
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	chain = append(chain, root)
	slices.Reverse(chain)
	return chain
}
