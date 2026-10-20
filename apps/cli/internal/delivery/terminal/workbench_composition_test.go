package terminal

import (
	"sync/atomic"

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

func openTestWorkbench(directory string) (*workbench.Store, error) {
	persistence, err := statefile.Open(directory)
	if err != nil {
		return nil, err
	}
	return workbench.Open(persistence, workbench.Config{})
}

func persistentTestWorkbench(directory string) func() (*workbench.Store, error) {
	return func() (*workbench.Store, error) { return openTestWorkbench(directory) }
}

func memoryTestWorkbench() (*workbench.Store, error) {
	return workbench.OpenMemory(workbench.Config{})
}
