package fileobservation

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"

	"github.com/Tangerg/flame/runtime/internal/infra/filesystem/pathidentity"
)

// observationRoots pins every configured physical boundary for one observer.
// Absolute paths retain their projection identity, while all confined I/O is
// addressed relative to the pinned root so a later symlink replacement cannot
// redirect a read outside that boundary.
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
		if openErr != nil {
			return nil, fmt.Errorf("open observation boundary %q: %w", boundary, openErr)
		}
		roots.byBoundary[boundary] = root
	}
	return roots, nil
}

func (r *observationRoots) access(boundary, physical string) (*os.Root, string, bool, error) {
	if boundary == "" {
		return nil, physical, true, nil
	}
	inside, err := pathidentity.Contains(boundary, physical)
	if err != nil {
		return nil, "", false, err
	}
	if !inside {
		return nil, "", false, nil
	}
	name, err := filepath.Rel(boundary, physical)
	if err != nil {
		return nil, "", false, err
	}
	root := r.byBoundary[boundary]
	if root == nil {
		return nil, "", false, fmt.Errorf("observation boundary %q is not open", boundary)
	}
	return root, name, true, nil
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
			if err := r.byBoundary[boundary].Close(); err != nil {
				r.closeErr = errors.Join(r.closeErr, fmt.Errorf("close observation boundary %q: %w", boundary, err))
			}
		}
	})
	return r.closeErr
}
