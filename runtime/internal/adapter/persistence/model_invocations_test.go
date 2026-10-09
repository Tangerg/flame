package persistence_test

import (
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/infra/sqlite"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/scope/core/chat"
)

func TestModelInvocationReaderValidatesStoredEvidence(t *testing.T) {
	for _, test := range []struct {
		name        string
		edit        string
		rejectWrite bool
	}{
		{name: "empty call identity", edit: "call_id = ''"},
		{name: "invalid effect identity", edit: "call_id = 'invalid effect'"},
		{name: "empty segment identity", edit: "segment_id = ''"},
		{name: "terminal without settlement", edit: "finished_at = NULL", rejectWrite: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			db, err := sqlite.Open(t.Context(), ":memory:")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Close() })
			at := time.Unix(100, 0).UTC()
			draft := run.Draft{
				RunID: "run_evidence", SessionID: "session_evidence", SegmentID: "segment_evidence",
				ModelSelection: testsupport.MustModelSelection("test", "test-model"), CreatedAt: at,
			}
			if err := sqlite.NewRunStore(db).Admit(t.Context(), draft); err != nil {
				t.Fatal(err)
			}
			store := sqlite.NewModelInvocationStore(db)
			if err := store.StartModelInvocation(t.Context(), draft.SessionID, draft.RunID, draft.SegmentID, "call_evidence", at); err != nil {
				t.Fatal(err)
			}
			if err := store.CompleteModelInvocation(t.Context(), draft.SessionID, draft.RunID, draft.SegmentID, "call_evidence", at, at.Add(time.Second), new(int64(0)), &chat.Usage{}); err != nil {
				t.Fatal(err)
			}
			reader, err := persistence.NewModelInvocationReader(store)
			if err != nil {
				t.Fatal(err)
			}
			rows, err := reader.PageModelInvocations(t.Context(), draft.RunID, 0, "", 10)
			if err != nil || len(rows) != 1 || rows[0].State != runs.ModelInvocationCompleted ||
				rows[0].Usage == nil || rows[0].FirstOutputLatencyMillis == nil || *rows[0].FirstOutputLatencyMillis != 0 {
				t.Fatalf("valid evidence = %+v, %v", rows, err)
			}
			_, editErr := db.ExecContext(t.Context(), "UPDATE model_invocations SET "+test.edit)
			if test.rejectWrite {
				if editErr == nil {
					t.Fatal("terminal attempt lost its required settlement time")
				}
				rows, err := reader.PageModelInvocations(t.Context(), draft.RunID, 0, "", 10)
				if err != nil || len(rows) != 1 || !rows[0].FinishedAt.Equal(at.Add(time.Second)) {
					t.Fatalf("refused corruption changed terminal evidence: %+v, %v", rows, err)
				}
				return
			}
			if editErr != nil {
				t.Fatal(editErr)
			}
			if rows, err := reader.PageModelInvocations(t.Context(), draft.RunID, 0, "", 10); err == nil || len(rows) != 0 {
				t.Fatalf("invalid evidence was projected: %+v, %v", rows, err)
			}
		})
	}
}
