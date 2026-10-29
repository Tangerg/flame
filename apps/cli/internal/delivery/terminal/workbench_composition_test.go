package terminal

import (
	"sync/atomic"
	"testing"

	"github.com/Tangerg/flame/apps/cli/internal/adapter/filesystem/statefile"
	"github.com/Tangerg/flame/apps/cli/internal/application/workbench"
)

func observeTestWorkbench(open func() (*workbench.Store, error)) (func() (*workbench.Store, error), func() *workbench.Store) {
	var owner atomic.Pointer[workbench.Store]
	return func() (*workbench.Store, error) {
		store, err := open()
		if err == nil {
			owner.Store(store)
		}
		return store, err
	}, owner.Load
}

func openTestWorkbench(t testing.TB, directory string) (*workbench.Store, error) {
	t.Helper()
	persistence, err := statefile.Open(directory)
	if err != nil {
		return nil, err
	}
	store, err := workbench.Open(persistence, workbench.Config{})
	if err == nil {
		t.Cleanup(func() {
			if err := store.Close(); err != nil {
				t.Error(err)
			}
		})
	}
	return store, err
}

func persistentTestWorkbench(t testing.TB, directory string) func() (*workbench.Store, error) {
	t.Helper()
	return func() (*workbench.Store, error) { return openTestWorkbench(t, directory) }
}

func memoryTestWorkbench() (*workbench.Store, error) {
	return workbench.OpenMemory(workbench.Config{})
}
