//go:build !darwin

package fileobservation

import "github.com/fsnotify/fsnotify"

func newPathObservation() (pathObservation, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return pathObservation{}, err
	}
	return pathObservation{registry: watcher, events: watcher.Events, errors: watcher.Errors}, nil
}
