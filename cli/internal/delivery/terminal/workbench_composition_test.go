package terminal

import (
	"github.com/Tangerg/flame/cli/internal/adapter/filesystem/statefile"
	"github.com/Tangerg/flame/cli/internal/application/workbench"
)

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
