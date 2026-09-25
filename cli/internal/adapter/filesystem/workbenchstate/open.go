// Package workbenchstate decides what a state directory means for authoring
// state. It is the one place that reads an empty directory as the session that
// keeps nothing on disk, so the command surface and the terminal surface cannot
// answer that differently for the same configuration.
//
// It lives in the adapter ring because it is the only ring that may see both
// collaborators: the workbench is an application use case and the state file is
// a filesystem adapter, and the application ring may not reach back out to one.
package workbenchstate

import (
	"strings"

	"github.com/Tangerg/flame/cli/internal/adapter/filesystem/statefile"
	"github.com/Tangerg/flame/cli/internal/application/agent/workbench"
)

// Open returns the authoring store for directory. An empty directory keeps
// nothing on disk; any other directory is backed by its state file.
func Open(directory string) (*workbench.Store, error) {
	if strings.TrimSpace(directory) == "" {
		return workbench.OpenMemory(workbench.Config{})
	}
	persistence, err := statefile.Open(directory)
	if err != nil {
		return nil, err
	}
	return workbench.Open(persistence, workbench.Config{})
}
