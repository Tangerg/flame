package workspace

import (
	"errors"
	"io"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
)

type recordingFileWatcher struct{ scopes []WatchScope }

func (r *recordingFileWatcher) Watch(scopes []WatchScope, _ func(ObservationChange), _ func(error)) (io.Closer, error) {
	r.scopes = slices.Clone(scopes)
	return nopCloser{}, nil
}

func TestWatchRejectsUnboundedAndEscapingTargets(t *testing.T) {
	watcher := &recordingFileWatcher{}
	w, err := NewWatch(newScope(t, "/root", "/home", testPaths{}), watcher)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		paths []string
		cause error
	}{
		{paths: make([]string, MaxWatchPaths+1), cause: ErrWatchLimit},
		{paths: []string{"../outside"}, cause: ErrPathOutsideRoot},
		{paths: []string{"/outside"}, cause: ErrPathOutsideRoot},
		{paths: []string{""}, cause: ErrPathRequired},
	} {
		observation, err := w.Watch([]WatchScope{{Key: "test", Paths: test.paths}}, func(ObservationChange) {}, func(error) {})
		if observation != nil || !errors.Is(err, test.cause) {
			t.Fatalf("Watch = (%v, %v), want %v", observation, err, test.cause)
		}
	}
	if watcher.scopes != nil {
		t.Fatal("rejected paths reached filesystem observation")
	}
}

type nopCloser struct{}

func (nopCloser) Close() error { return nil }

func TestNewWatchRequiresCompleteDependencies(t *testing.T) {
	scope := newScope(t, "", "", testPaths{})
	for _, test := range []struct {
		name    string
		scope   *Scope
		watcher FileWatcher
	}{
		{name: "scope", watcher: &recordingFileWatcher{}},
		{name: "watcher", scope: scope},
		{name: "typed nil watcher", scope: scope, watcher: (*recordingFileWatcher)(nil)},
	} {
		t.Run(test.name, func(t *testing.T) {
			if watch, err := NewWatch(test.scope, test.watcher); err == nil || watch != nil {
				t.Fatalf("NewWatch = (%v, %v), want incomplete construction rejected", watch, err)
			}
		})
	}
}

func TestWatchResolvesAndOwnsObservationRoots(t *testing.T) {
	root := t.TempDir()
	scopes := []WatchScope{{Key: "default"}, {Key: "root", Root: root}, {Key: "second", Root: filepath.Join(root, "second")}}
	watcher := &recordingFileWatcher{}
	useCases, err := NewWatch(newScope(t, root, root, testPaths{}), watcher)
	if err != nil {
		t.Fatal(err)
	}

	observation, err := useCases.Watch(scopes, func(ObservationChange) {}, func(error) {})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = observation.Close() }()
	scopes[2].Root = filepath.Join(root, "changed")
	want := []WatchScope{{Key: "default", Root: root}, {Key: "root", Root: root}, {Key: "second", Root: filepath.Join(root, "second")}}
	if !reflect.DeepEqual(watcher.scopes, want) {
		t.Fatalf("Git roots after caller reused input = %v, want %v", watcher.scopes, want)
	}
}
