package execution

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/testsupport/persistencefixture"
	"github.com/Tangerg/scope/agent"
	"github.com/Tangerg/scope/agent/agenttest"
)

func TestSQLiteTreeCommitterConformance(t *testing.T) {
	agenttest.RunTreeCommitterConformance(t, func() agenttest.TreeCommitterConformanceDriver {
		db, err := persistencefixture.Open(t, t.Context(), filepath.Dir(filepath.Join(t.TempDir(), "tree.db")))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := db.Close(); err != nil {
				t.Error(err)
			}
		})
		driver := &sqliteTreeCommitterDriver{interactionSession: &interactionSession{
			start: runs.RootExecutionStart{SessionID: "session"}, lifetime: newInteractionLifetime(t.Context()),
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

type sqliteTreeCommitterDriver struct{ *interactionSession }

func (d *sqliteTreeCommitterDriver) LoadTree(ctx context.Context, rootID agent.ProcessID) (agent.TreeSnapshot, bool, error) {
	head, found, err := d.executionTrees.LoadExecutionTree(ctx, d.start.SessionID, rootID.String())
	if err != nil || !found {
		return agent.TreeSnapshot{}, found, err
	}
	tree, err := decodeExecutionTree(head, rootID)
	return tree, err == nil, err
}
