package workspace

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"slices"

	"github.com/Tangerg/flame/runtime/internal/dependency"
)

const (
	MaxWatchPaths                  = 256
	MaxWatchDirectoryEntries       = 10000
	MaxWatchFileBytes        int64 = 1 << 20
)

var ErrWatchLimit = errors.New("workspace: file observation limit exceeded")

// WatchScope names one workspace and exact paths relative to it. Key belongs to
// the subscriber. An empty Paths set requests only Git metadata observation.
type WatchScope struct {
	Key   string
	Root  string
	Paths []string
}

type ObservationChange struct {
	Key   string
	Root  string
	Paths []string
}

// FileWatcher owns bounded filesystem observation. A notification with no paths
// invalidates the whole named workspace. A reported failure requires rebuilding
// the observation; Close joins both callbacks.
type FileWatcher interface {
	Watch(scopes []WatchScope, notify func(ObservationChange), report func(error)) (io.Closer, error)
}

type Watch struct {
	scope   *Scope
	watcher FileWatcher
}

func NewWatch(scope *Scope, watcher FileWatcher) (*Watch, error) {
	if scope == nil {
		return nil, errors.New("workspace: watch scope is required")
	}
	if dependency.Missing(watcher) {
		return nil, errors.New("workspace: file watcher is required")
	}
	return &Watch{scope: scope, watcher: watcher}, nil
}

func (w *Watch) Watch(scopes []WatchScope, notify func(ObservationChange), report func(error)) (io.Closer, error) {
	total := 0
	resolved := make([]WatchScope, 0, len(scopes))
	for _, candidate := range scopes {
		total += len(candidate.Paths)
		if total > MaxWatchPaths {
			return nil, fmt.Errorf("%w: at most %d paths per subscription", ErrWatchLimit, MaxWatchPaths)
		}
		root, err := w.scope.root(candidate.Root)
		if err != nil {
			return nil, err
		}
		next := WatchScope{Key: candidate.Key, Root: root}
		for _, path := range candidate.Paths {
			if path == "" {
				return nil, ErrPathRequired
			}
			if !filepath.IsLocal(path) {
				return nil, ErrPathOutsideRoot
			}
			if _, err := w.scope.paths.ResolveExistingInRoot(root, path); err != nil {
				return nil, err
			}
			path = filepath.ToSlash(filepath.Clean(path))
			if !slices.Contains(next.Paths, path) {
				next.Paths = append(next.Paths, path)
			}
		}
		resolved = append(resolved, next)
	}
	return w.watcher.Watch(resolved, notify, report)
}
