package persistence

import (
	"context"
	"strings"
	"testing"

	framesqlite "github.com/Tangerg/frame/sqlite"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

// seedParkedRuns writes the Runs a test hand-off names. Every child member is a
// direct child of the root, spawned by "item_spawn_" plus its Run suffix.
func seedParkedRuns(t *testing.T, db *framesqlite.Database, sessionID string, pending runs.Pending) {
	t.Helper()
	members := make([]testsupport.ParkedMember, len(pending.Continuations))
	for index, continuation := range pending.Continuations {
		members[index] = testsupport.ParkedMember{RunID: continuation.RunID}
		if continuation.RunID != pending.RootRunID {
			members[index].Lineage = testsupport.ChildLineage("item_spawn_"+strings.TrimPrefix(continuation.RunID, "run_"), pending.RootRunID, pending.RootRunID)
		}
	}
	if err := framesqlite.RunInTx(t.Context(), db, func(ctx context.Context) error {
		testsupport.SeedParkedRuns(t, framesqlite.Connection(ctx, db), sessionID, pending.RootRunID, "", run.Capabilities{}, members)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
