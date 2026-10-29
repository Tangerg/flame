package run

import (
	"testing"

	"github.com/Tangerg/flame/apps/cli/internal/adapter/filesystem/statefile"
	"github.com/Tangerg/flame/apps/cli/internal/application/workbench"
)

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
