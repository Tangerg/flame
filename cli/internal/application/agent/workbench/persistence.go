package workbench

import (
	"errors"
	"path/filepath"
	"strings"
)

type persistenceKind uint8

const (
	memoryPersistence persistenceKind = iota + 1
	directoryPersistence
)

// persistence is the immutable storage capability of one Store. Its zero
// value is invalid so an empty path cannot silently turn durable state into
// process memory.
type persistence struct {
	kind      persistenceKind
	directory string
}

func newMemoryPersistence() persistence {
	return persistence{kind: memoryPersistence}
}

func newDirectoryPersistence(directory string) (persistence, error) {
	directory = strings.TrimSpace(directory)
	if directory == "" {
		return persistence{}, errors.New("workbench directory is empty")
	}
	if !filepath.IsAbs(directory) {
		return persistence{}, errors.New("workbench directory must be absolute")
	}
	return persistence{kind: directoryPersistence, directory: filepath.Clean(directory)}, nil
}

func (p persistence) Validate() error {
	switch p.kind {
	case memoryPersistence:
		if p.directory != "" {
			return errors.New("memory workbench carries a directory")
		}
		return nil
	case directoryPersistence:
		if strings.TrimSpace(p.directory) == "" || !filepath.IsAbs(p.directory) || filepath.Clean(p.directory) != p.directory {
			return errors.New("directory workbench path is invalid")
		}
		return nil
	default:
		return errors.New("workbench persistence is not configured")
	}
}

func (p persistence) Durable() bool { return p.kind == directoryPersistence }

func (p persistence) Directory() string { return p.directory }
