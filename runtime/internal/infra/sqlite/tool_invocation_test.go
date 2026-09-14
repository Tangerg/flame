package sqlite_test

import (
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
	runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"
	"github.com/Tangerg/flame/runtime/internal/infra/sqlite"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func TestToolInvocationJournalAllowsOneLogicalCallAcrossContinuationSegments(t *testing.T) {
	db, err := sqlite.Open(t.Context(), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	segments := []string{"segment_before_wait", "segment_after_answer"}
	runs := sqlite.NewRunStore(db)
	if err := runs.Admit(t.Context(), testsupport.RunDraft(run.Draft{
		RunID: "run_1", SessionID: "session_1", SegmentID: segments[0], CreatedAt: time.Now().UTC(),
	})); err != nil {
		t.Fatalf("admit Run: %v", err)
	}
	store := sqlite.NewToolInvocationStore(db)
	startedAt := time.Now().UTC()
	for index, segmentID := range segments {
		if index > 0 {
			active, found, err := runs.Run(t.Context(), "run_1")
			if err != nil || !found {
				t.Fatalf("active Run: %t, %v", found, err)
			}
			waiting, err := active.Suspend(startedAt.Add(time.Millisecond))
			if err != nil {
				t.Fatal(err)
			}
			if err := runs.Suspend(t.Context(), waiting, segments[0], runtimeidentity.CommitID{}); err != nil {
				t.Fatal(err)
			}
			startedAt = startedAt.Add(2 * time.Millisecond)
			if err := runs.Resume(t.Context(), "session_1", run.ResumeDraft{RunID: "run_1", SegmentID: segmentID}, startedAt); err != nil {
				t.Fatal(err)
			}
		}
		if err := store.StartToolInvocation(
			t.Context(), "session_1", "run_1", segmentID,
			"logical_call_1", "item_1", startedAt,
		); err != nil {
			t.Fatalf("start %s: %v", segmentID, err)
		}
		finish := store.CompleteToolInvocation
		if index == 0 {
			finish = store.MarkToolInvocationIncomplete
		}
		if err := finish(
			t.Context(), "session_1", "run_1", segmentID, "logical_call_1", "item_1",
			startedAt, startedAt.Add(time.Millisecond),
		); err != nil {
			t.Fatalf("complete %s: %v", segmentID, err)
		}
	}
	var rows int
	if err := db.QueryRowContext(t.Context(), `
		SELECT count(*) FROM tool_invocations
		 WHERE call_id = ? AND item_id = ? AND state IN ('incomplete', 'completed')
	`, "logical_call_1", "item_1").Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 2 {
		t.Fatalf("journal rows = %d, want 2 segment-owned attempts", rows)
	}
}
