package testsupport

import (
	"context"
	"database/sql"
	json "encoding/json/v2"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
)

// ParkedMember is one waiting Run a test hand-off names.
type ParkedMember struct {
	RunID   string
	Lineage run.Lineage
}

type StatementExecutor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

// SeedParkedRuns gives a test hand-off the Runs that own its facts. A hand-off
// stores none of them: each member Run absent from the store is written waiting
// with the root's Goal incarnation and capabilities and its own lineage; a Run
// the test already wrote keeps its own facts.
func SeedParkedRuns(
	t testing.TB,
	db StatementExecutor,
	sessionID, rootRunID, goalIncarnationID string,
	capabilities run.Capabilities,
	members []ParkedMember,
) {
	t.Helper()
	encoded := ""
	if !capabilities.IsEmpty() {
		body, err := json.Marshal(struct {
			ChildRuns      bool                `json:"childRuns,omitzero"`
			InterruptTypes []run.InterruptType `json:"interruptTypes,omitempty"`
		}{capabilities.ChildRuns, capabilities.InterruptTypes})
		if err != nil {
			t.Fatal(err)
		}
		encoded = string(body)
	}
	for _, member := range members {
		goal, memberCapabilities := "", ""
		if member.RunID == rootRunID {
			goal, memberCapabilities = goalIncarnationID, encoded
		}
		if _, err := db.ExecContext(t.Context(),
			`INSERT INTO runs(run_id, session_id, spawned_by_item_id, parent_run_id, root_run_id,
			                  state, provider, model, goal_incarnation_id, capabilities, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, 'waiting', ?, ?, ?, ?, 0, 0)
			 ON CONFLICT(run_id) DO NOTHING`,
			member.RunID, sessionID,
			member.Lineage.SpawnedByItemID(), member.Lineage.ParentRunID(), member.Lineage.RootRunID(),
			DefaultModelSelection().Provider(), DefaultModelSelection().Model(),
			goal, memberCapabilities,
		); err != nil {
			t.Fatalf("seed parked Run %q: %v", member.RunID, err)
		}
	}
}
