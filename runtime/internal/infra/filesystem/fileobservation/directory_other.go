//go:build !darwin

package fileobservation

import "github.com/fsnotify/fsnotify"

func newDirectoryObservation() (directoryObservation, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return directoryObservation{}, err
	}
	return directoryObservation{registry: watcher, events: watcher.Events, errors: watcher.Errors}, nil
}
