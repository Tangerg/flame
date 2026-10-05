package sqlite_test

import (
	"database/sql"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func seedParkedRuns(t *testing.T, db *sql.DB, pending runs.Pending) {
	t.Helper()
	testsupport.SeedParkedRuns(t, db, pending.SessionID, pending.RootRunID, pending.GoalIncarnationID,
		pending.Capabilities, parkedMembers(pending))
}

func parkedMembers(pending runs.Pending) []testsupport.ParkedMember {
	members := make([]testsupport.ParkedMember, len(pending.Continuations))
	for index, continuation := range pending.Continuations {
		members[index] = testsupport.ParkedMember{RunID: continuation.RunID, Lineage: continuation.Lineage}
	}
	return members
}
