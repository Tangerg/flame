package sqlite_test

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

// seedParkedRuns writes the Runs a test hand-off names. Every child member is a
// direct child of the root, spawned by "item_spawn_" plus its Run suffix.
func seedParkedRuns(t *testing.T, db *sql.DB, sessionID string, pending runs.Pending) {
	t.Helper()
	members := make([]testsupport.ParkedMember, len(pending.Continuations))
	for index, continuation := range pending.Continuations {
		members[index] = testsupport.ParkedMember{RunID: continuation.RunID}
		if continuation.RunID != pending.RootRunID {
			members[index].Lineage = run.Lineage{
				SpawnedByItemID: "item_spawn_" + strings.TrimPrefix(continuation.RunID, "run_"),
				ParentRunID:     pending.RootRunID,
				RootRunID:       pending.RootRunID,
			}
		}
	}
	testsupport.SeedParkedRuns(t, db, sessionID, pending.RootRunID, "", run.Capabilities{}, members)
}
