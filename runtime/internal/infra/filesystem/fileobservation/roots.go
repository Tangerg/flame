package fileobservation

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"

	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
)

type observationRoots struct {
	byBoundary map[string]*os.Root
	closeOnce  sync.Once
	closeErr   error
}

func openObservationRoots(boundaries []string) (_ *observationRoots, err error) {
	roots := &observationRoots{byBoundary: make(map[string]*os.Root)}
	defer func() {
		if err != nil {
			err = errors.Join(err, roots.Close())
		}
	}()
	for _, boundary := range boundaries {
		if boundary == "" || roots.byBoundary[boundary] != nil {
			continue
		}
		root, openErr := os.OpenRoot(boundary)
		if openErr != nil && !errors.Is(openErr, os.ErrNotExist) {
			return nil, fmt.Errorf("open observation boundary %q: %w", boundary, openErr)
		}
		roots.byBoundary[boundary] = root
	}
	return roots, nil
}

type observedName struct {
	root *os.Root
	name string
}

func (n observedName) stat() (os.FileInfo, error) {
	if n.root != nil {
		return n.root.Stat(n.name)
	}
	return os.Stat(n.name)
}

func (n observedName) lstat() (os.FileInfo, error) {
	if n.root != nil {
		return n.root.Lstat(n.name)
	}
	return os.Lstat(n.name)
}

func (n observedName) open() (*os.File, error) {
	if n.root != nil {
		return n.root.Open(n.name)
	}
	return os.Open(n.name)
}

func (n observedName) openFile(expected os.FileInfo, maxBytes int64) (*os.File, os.FileInfo, error) {
	if n.root != nil {
		return filesystem.OpenFileAtExpected(n.root, n.name, expected, maxBytes)
	}
	return filesystem.OpenFileExpected(n.name, expected, maxBytes)
}

func (n observedName) openDirectory(expected os.FileInfo) (*os.File, os.FileInfo, error) {
	if n.root != nil {
		return filesystem.OpenDirectoryAtExpected(n.root, n.name, expected)
	}
	return filesystem.OpenDirectoryExpected(n.name, expected)
}

func (n observedName) child(entry string) observedName {
	return observedName{root: n.root, name: filepath.Join(n.name, entry)}
}

func (r *observationRoots) access(boundary, physical string) (observedName, bool, error) {
	if boundary == "" {
		return observedName{name: physical}, true, nil
	}
	inside, err := filesystem.ContainsPath(boundary, physical)
	if err != nil {
		return observedName{}, false, err
	}
	if !inside {
		return observedName{}, false, nil
	}
	name, err := filepath.Rel(boundary, physical)
	if err != nil {
		return observedName{}, false, err
	}
	root := r.byBoundary[boundary]
	if root != nil {
		named, err := os.Stat(boundary)
		if errors.Is(err, os.ErrNotExist) {
			return observedName{}, false, nil
		}
		if err != nil {
			return observedName{}, false, fmt.Errorf("inspect observation boundary %q: %w", boundary, err)
		}
		pinned, err := root.Stat(".")
		if err != nil {
			return observedName{}, false, fmt.Errorf("inspect pinned observation boundary %q: %w", boundary, err)
		}
		if !os.SameFile(named, pinned) {
			// Reconciliation must not mix a new pathname with the old directory
			// handle. Report the lost binding before a retry can pin its successor.
			r.byBoundary[boundary] = nil
			return observedName{}, false, errors.Join(fmt.Errorf("observation boundary %q was replaced", boundary), root.Close())
		}
	}
	if root == nil {
		root, err = os.OpenRoot(boundary)
		if errors.Is(err, os.ErrNotExist) {
			return observedName{}, false, nil
		}
		if err != nil {
			return observedName{}, false, fmt.Errorf("open observation boundary %q: %w", boundary, err)
		}
		r.byBoundary[boundary] = root
	}
	return observedName{root: root, name: name}, true, nil
}

func (r *observationRoots) Close() error {
	if r == nil {
		return nil
	}
	r.closeOnce.Do(func() {
		boundaries := make([]string, 0, len(r.byBoundary))
		for boundary := range r.byBoundary {
			boundaries = append(boundaries, boundary)
		}
		slices.Sort(boundaries)
		for _, boundary := range boundaries {
			if r.byBoundary[boundary] == nil {
				continue
			}
			if err := r.byBoundary[boundary].Close(); err != nil {
				r.closeErr = errors.Join(r.closeErr, fmt.Errorf("close observation boundary %q: %w", boundary, err))
			}
		}
	})
	return r.closeErr
}
