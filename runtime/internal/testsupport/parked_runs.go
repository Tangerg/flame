package testsupport

import (
	"database/sql"
	json "encoding/json/v2"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/interrupt"
)

// ParkedMember is one waiting Run a test hand-off names.
type ParkedMember struct {
	RunID   string
	Lineage run.Lineage
}

// SeedParkedRuns gives a test hand-off the Runs that own its facts. A hand-off
// stores none of them: each member Run is written waiting when absent, and the
// root's Goal incarnation and capabilities and every member's lineage become
// the ones the fixture names.
func SeedParkedRuns(
	t testing.TB,
	db *sql.DB,
	sessionID, rootRunID, goalIncarnationID string,
	capabilities run.Capabilities,
	members []ParkedMember,
) {
	t.Helper()
	encoded := ""
	if !capabilities.IsEmpty() {
		body, err := json.Marshal(struct {
			ChildRuns      bool             `json:"childRuns,omitzero"`
			InterruptKinds []interrupt.Kind `json:"interruptKinds,omitempty"`
		}{capabilities.ChildRuns, capabilities.InterruptKinds})
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
			                  state, goal_incarnation_id, capabilities, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, 'waiting', ?, ?, 0, 0)
			 ON CONFLICT(run_id) DO UPDATE SET
			   spawned_by_item_id = excluded.spawned_by_item_id,
			   parent_run_id = excluded.parent_run_id,
			   root_run_id = excluded.root_run_id,
			   goal_incarnation_id = excluded.goal_incarnation_id,
			   capabilities = excluded.capabilities`,
			member.RunID, sessionID,
			member.Lineage.SpawnedByItemID, member.Lineage.ParentRunID, member.Lineage.RootRunID,
			goal, memberCapabilities,
		); err != nil {
			t.Fatalf("seed parked Run %q: %v", member.RunID, err)
		}
	}
}
