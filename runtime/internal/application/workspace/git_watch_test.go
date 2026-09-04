package workspace

import (
	"io"
	"path/filepath"
	"reflect"
	"testing"
)

type callbackPaths struct {
	onFirst func()
	calls   int
}

func (c *callbackPaths) ResolveExistingDir(path string) (string, error) {
	c.calls++
	if c.calls == 1 && c.onFirst != nil {
		c.onFirst()
	}
	return path, nil
}

func (*callbackPaths) ResolveInRoot(_, path string) (string, error) { return path, nil }
func (*callbackPaths) ResolveExistingInRoot(_, path string) (string, error) {
	return path, nil
}

type recordingGitWatcher struct{ roots []string }

func (r *recordingGitWatcher) Watch(roots []string, _ func()) (io.Closer, error) {
	r.roots = roots
	return nopCloser{}, nil
}

type nopCloser struct{}

func (nopCloser) Close() error { return nil }

func TestGitWatchOwnsCWDsBeforePathResolution(t *testing.T) {
	root := t.TempDir()
	cwds := []string{filepath.Join(root, "first"), filepath.Join(root, "second")}
	paths := &callbackPaths{onFirst: func() { cwds[1] = filepath.Join(root, "changed") }}
	watcher := &recordingGitWatcher{}
	useCases := NewGitWatch(NewScope(root, root, paths), watcher)

	observation, err := useCases.Watch(cwds, func() {})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = observation.Close() }()
	want := []string{filepath.Join(root, "first"), filepath.Join(root, "second")}
	if !reflect.DeepEqual(watcher.roots, want) {
		t.Fatalf("Git roots after resolver changed caller input = %v, want %v", watcher.roots, want)
	}
}
