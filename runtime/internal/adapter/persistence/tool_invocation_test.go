package persistence

import (
	"path/filepath"
	"testing"
	"time"

	flameruns "github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func TestToolInvocationJournalAllowsOneLogicalCallAcrossContinuationSegments(t *testing.T) {
	path := filepath.Join(t.TempDir(), "attempts.sqlite")
	db, err := openDatabase(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	segments := []string{"segment_before_wait", "segment_after_answer"}
	runs := newRunStore(db)
	createdAt := time.Unix(100, 0).UTC()
	if err := runs.Admit(t.Context(), testsupport.RunDraft(run.Draft{
		RunID: "run_1", SessionID: "session_1", SegmentID: segments[0], CreatedAt: createdAt,
	})); err != nil {
		t.Fatalf("admit Run: %v", err)
	}
	store := newToolInvocationStore(db)
	startedAt := createdAt.Add(time.Second)
	starts := make([]time.Time, len(segments))
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
			if err := runs.Suspend(t.Context(), waiting, segments[0], flameruns.CommitID{}); err != nil {
				t.Fatal(err)
			}
			startedAt = startedAt.Add(2 * time.Millisecond)
			if err := resumeParkedRun(t.Context(), runs, waiting, segmentID, startedAt); err != nil {
				t.Fatal(err)
			}
		}
		if err := store.StartToolInvocation(
			t.Context(), "session_1", "run_1", segmentID,
			"logical_call_1", "item_1", startedAt,
		); err != nil {
			t.Fatalf("start %s: %v", segmentID, err)
		}
		starts[index] = startedAt
		if err := store.StartToolInvocation(
			t.Context(), "session_1", "run_1", segmentID,
			"logical_call_1", "item_1", startedAt.Add(time.Microsecond),
		); err == nil {
			t.Fatalf("replaced the original start of %s", segmentID)
		}
		finish := store.CompleteToolInvocation
		if index == 0 {
			finish = store.MarkToolInvocationIncomplete
		}
		if err := finish(
			t.Context(), "session_1", "run_1", segmentID, "logical_call_1", "item_1",
			startedAt.Add(time.Microsecond), startedAt.Add(time.Millisecond),
		); err == nil {
			t.Fatalf("settled a different start of %s", segmentID)
		}
		if err := finish(
			t.Context(), "session_1", "run_1", segmentID, "logical_call_1", "item_1",
			startedAt, startedAt.Add(time.Millisecond),
		); err != nil {
			t.Fatalf("complete %s: %v", segmentID, err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = openDatabase(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	runs = newRunStore(db)
	active, found, err := runs.Run(t.Context(), "run_1")
	if err != nil || !found {
		t.Fatalf("active Run = %v, %v", found, err)
	}
	terminal, err := active.Terminate(run.Termination{
		Outcome: run.OutcomeCompleted, FinishedAt: startedAt.Add(time.Second), MessageMark: run.MessageMarkAt(0),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := runs.Terminalize(t.Context(), storedRunReplacement(t, t.Context(), runs, terminal)); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = openDatabase(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	store = newToolInvocationStore(db)
	rows, err := store.ListSession(t.Context(), "session_1")
	if err != nil || len(rows) != len(segments) {
		t.Fatalf("retained attempts = %+v, %v", rows, err)
	}
	for index, row := range rows {
		state := "completed"
		if index == 0 {
			state = "incomplete"
		}
		if row.CallID != "logical_call_1" || row.ItemID != "item_1" || row.RunID != "run_1" ||
			row.SegmentID != segments[index] || row.State != state || !row.StartedAt.Equal(starts[index]) ||
			!row.FinishedAt.Equal(starts[index].Add(time.Millisecond)) {
			t.Fatalf("attempt %d = %+v", index, row)
		}
	}
	if other, err := store.ListSession(t.Context(), "session_other"); err != nil || len(other) != 0 {
		t.Fatalf("foreign Session attempts = %+v, %v", other, err)
	}
	if err := newRunStore(db).Delete(t.Context(), "session_1", "run_1"); err != nil {
		t.Fatal(err)
	}
	if deleted, err := store.ListSession(t.Context(), "session_1"); err != nil || len(deleted) != 0 {
		t.Fatalf("deleted Run attempts = %+v, %v", deleted, err)
	}
}

func TestToolInvocationReadRetainsUnsettledTerminalEvidence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "unsettled.sqlite")
	db, err := openDatabase(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	runs := newRunStore(db)
	draft := runDraft("run_unsettled", "session_unsettled")
	if err := runs.Admit(t.Context(), draft); err != nil {
		t.Fatal(err)
	}
	store := newToolInvocationStore(db)
	if err := store.StartToolInvocation(t.Context(), draft.SessionID, draft.RunID, draft.SegmentID,
		"call_unsettled", "item_unsettled", draft.CreatedAt); err != nil {
		t.Fatal(err)
	}
	if err := runs.Terminalize(t.Context(), storedRunReplacement(t, t.Context(), runs, finishedRunFromDraft(draft, run.OutcomeCompleted))); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = openDatabase(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := newToolInvocationStore(db).ListSession(t.Context(), draft.SessionID)
	if err != nil || len(rows) != 1 || rows[0].State != "started" || !rows[0].FinishedAt.IsZero() ||
		!rows[0].StartedAt.Equal(draft.CreatedAt) {
		t.Fatalf("unsettled terminal evidence = %+v, %v", rows, err)
	}
}

func TestToolInvocationReadRejectsForeignOwnershipWithoutPartialEvidence(t *testing.T) {
	db, err := openDatabase(t.Context(), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := newToolInvocationStore(db)
	for _, suffix := range []string{"a", "b"} {
		draft := runDraft("run_"+suffix, "session_"+suffix)
		if err := newRunStore(db).Admit(t.Context(), draft); err != nil {
			t.Fatal(err)
		}
		if err := store.StartToolInvocation(t.Context(), draft.SessionID, draft.RunID, draft.SegmentID,
			"call_"+suffix, "item_"+suffix, draft.CreatedAt); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.ExecContext(t.Context(), `UPDATE tool_invocations SET session_id = 'session_a' WHERE call_id = 'call_b'`); err != nil {
		t.Fatal(err)
	}
	if rows, err := store.ListSession(t.Context(), "session_a"); err == nil || len(rows) != 0 {
		t.Fatalf("mismatched ownership returned evidence = %+v, %v", rows, err)
	}
}
