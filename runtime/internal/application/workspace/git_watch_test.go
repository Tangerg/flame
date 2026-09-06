package workspace

import (
	"io"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
)

type recordingGitWatcher struct{ roots []string }

func (r *recordingGitWatcher) Watch(roots []string, _ func()) (io.Closer, error) {
	r.roots = slices.Clone(roots)
	return nopCloser{}, nil
}

type nopCloser struct{}

func (nopCloser) Close() error { return nil }

func TestNewGitWatchRequiresCompleteDependencies(t *testing.T) {
	scope := newScope(t, "", "", testPaths{})
	for _, test := range []struct {
		name    string
		scope   *Scope
		watcher GitStateWatcher
	}{
		{name: "scope", watcher: &recordingGitWatcher{}},
		{name: "watcher", scope: scope},
		{name: "typed nil watcher", scope: scope, watcher: (*recordingGitWatcher)(nil)},
	} {
		t.Run(test.name, func(t *testing.T) {
			if watch, err := NewGitWatch(test.scope, test.watcher); err == nil || watch != nil {
				t.Fatalf("NewGitWatch = (%v, %v), want incomplete construction rejected", watch, err)
			}
		})
	}
}

func TestGitWatchResolvesAndOwnsObservationRoots(t *testing.T) {
	root := t.TempDir()
	cwds := []string{"", root, filepath.Join(root, "second")}
	watcher := &recordingGitWatcher{}
	useCases, err := NewGitWatch(newScope(t, root, root, testPaths{}), watcher)
	if err != nil {
		t.Fatal(err)
	}

	observation, err := useCases.Watch(cwds, func() {})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = observation.Close() }()
	cwds[2] = filepath.Join(root, "changed")
	want := []string{root, filepath.Join(root, "second")}
	if !reflect.DeepEqual(watcher.roots, want) {
		t.Fatalf("Git roots after caller reused input = %v, want %v", watcher.roots, want)
	}
}
