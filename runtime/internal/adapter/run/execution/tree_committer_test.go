package execution

import (
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/testsupport/persistencefixture"

	scoperun "github.com/Tangerg/scope/agent/run"
	"github.com/Tangerg/scope/agent/run/runtest"
)

func TestSQLiteScopeStoreConformance(t *testing.T) {
	runtest.RunStoreConformance(t, func(t *testing.T) scoperun.Store {
		db, err := persistencefixture.Open(t, t.Context(), filepath.Dir(filepath.Join(t.TempDir(), "tree.db")))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := db.Close(); err != nil {
				t.Error(err)
			}
		})
		driver := &sqliteScopeStore{interactionSession: &interactionSession{
			start: runs.RootExecutionStart{SessionID: "session"}, lifetime: newInteractionLifetime(t.Context()),
			state:    interactionState{deployments: &interactionDeploymentSet{}},
			material: newRequestMaterial(),
			executionTrees: &scopeCommitCheckingStore{
				ExecutionTreeStore: db.ExecutorCheckpoints,
			},
		}}
		t.Cleanup(driver.lifetime.stopRelease)
		t.Cleanup(driver.lifetime.stopReconciling)
		t.Cleanup(driver.lifetime.stopExecution)
		return driver
	})
}

type sqliteScopeStore struct{ *interactionSession }
