//go:build !darwin

package fileobservation

import (
	"os"
	"path/filepath"

	"github.com/fsnotify/fsnotify"
)

func metadataWatchPath(path string, mode os.FileMode) string {
	if mode&os.ModeSymlink != 0 {
		return filepath.Dir(path)
	}
	return path
}

func newPathObservation() (pathObservation, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return pathObservation{}, err
	}
	return pathObservation{registry: watcher, events: watcher.Events, errors: watcher.Errors}, nil
}
