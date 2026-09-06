package workspace

import (
	"errors"
	"io"
)

// GitStateWatcher owns filesystem notification, debounce, repository layout,
// and goroutine lifetime for Git metadata changes. Watch borrows roots for the
// call; the implementation owns any retained observation configuration.
type GitStateWatcher interface {
	Watch(roots []string, notify func()) (io.Closer, error)
}

// GitWatch resolves requested workspaces before delegating technical watching.
type GitWatch struct {
	scope   *Scope
	watcher GitStateWatcher
}

func NewGitWatch(scope *Scope, watcher GitStateWatcher) (*GitWatch, error) {
	if scope == nil {
		return nil, errors.New("workspace: git watch scope is required")
	}
	if missingDependency(watcher) {
		return nil, errors.New("workspace: git state watcher is required")
	}
	return &GitWatch{scope: scope, watcher: watcher}, nil
}

// Watch borrows cwds while canonicalizing and deduplicating workspace roots.
func (g *GitWatch) Watch(cwds []string, notify func()) (io.Closer, error) {
	seen := make(map[string]struct{}, len(cwds))
	roots := make([]string, 0, len(cwds))
	for _, cwd := range cwds {
		root, err := g.scope.root(cwd)
		if err != nil {
			return nil, err
		}
		if _, duplicate := seen[root]; duplicate {
			continue
		}
		seen[root] = struct{}{}
		roots = append(roots, root)
	}
	return g.watcher.Watch(roots, notify)
}
