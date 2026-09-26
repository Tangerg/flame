package workspace

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strconv"

	workspaceapp "github.com/Tangerg/flame/runtime/internal/application/workspace"
	"github.com/Tangerg/flame/runtime/internal/infra/filesystem/fileobservation"
)

type FileWatcher struct{ git GitWatcher }

func NewFileWatcher(lifetime context.Context) FileWatcher {
	return FileWatcher{git: NewGitWatcher(lifetime)}
}

func (w FileWatcher) Watch(scopes []workspaceapp.WatchScope, notify func(workspaceapp.ObservationChange), report func(error)) (io.Closer, error) {
	if notify == nil || report == nil {
		return nil, errors.New("workspace: observation callbacks are required")
	}
	targets := make([]fileobservation.Target, 0)
	changes := make(map[string]workspaceapp.ObservationChange)
	rootScopes := make(map[string][]workspaceapp.ObservationChange)
	roots := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		if !slices.Contains(roots, scope.Root) {
			roots = append(roots, scope.Root)
		}
		rootScopes[scope.Root] = append(rootScopes[scope.Root], workspaceapp.ObservationChange{Key: scope.Key, Root: scope.Root})
		for _, path := range scope.Paths {
			key := strconv.Itoa(len(targets))
			targets = append(targets, fileobservation.Target{
				Key: key, Path: filepath.Join(scope.Root, path), Boundary: scope.Root,
				MaxBytes: workspaceapp.MaxWatchFileBytes, MaxEntries: workspaceapp.MaxWatchDirectoryEntries,
			})
			changes[key] = workspaceapp.ObservationChange{Key: scope.Key, Root: scope.Root, Paths: []string{path}}
		}
	}
	files, err := fileobservation.Watch(targets, func(keys []string) {
		batch := make(map[string]workspaceapp.ObservationChange)
		for _, key := range keys {
			change := changes[key]
			previous := batch[change.Key]
			change.Paths = append(previous.Paths, change.Paths...)
			batch[change.Key] = change
		}
		for _, scope := range scopes {
			if change, changed := batch[scope.Key]; changed {
				notify(change)
			}
		}
	}, report)
	if err != nil {
		return nil, fmt.Errorf("observe workspace paths: %w", err)
	}
	git, err := w.git.Watch(roots, func(changed []string) {
		for _, root := range changed {
			for _, scope := range rootScopes[root] {
				notify(scope)
			}
		}
	}, report)
	if err != nil {
		return nil, errors.Join(err, files.Close())
	}
	return fileWatch{files: files, git: git}, nil
}

type fileWatch struct{ files, git io.Closer }

func (w fileWatch) Close() error { return errors.Join(w.files.Close(), w.git.Close()) }
