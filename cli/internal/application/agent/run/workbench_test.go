package run

import (
	"github.com/Tangerg/flame/cli/internal/adapter/filesystem/statefile"
	"github.com/Tangerg/flame/cli/internal/application/agent/workbench"
)

func openTestWorkbench(directory string) (*workbench.Store, error) {
	persistence, err := statefile.Open(directory)
	if err != nil {
		return nil, err
	}
	return workbench.Open(persistence, workbench.Config{})
}
