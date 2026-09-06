package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func seedWorkspaceMutationSession(t *testing.T, db *sql.DB, sessionID, cwd string) {
	t.Helper()
	if err := NewSessionStore(db).Insert(t.Context(), testsupport.MustRestoreSession(session.Snapshot{
		ID: sessionID, Workspace: testsupport.MustWorkspace(cwd),
		StartedAt: time.Unix(1, 0), UpdatedAt: time.Unix(1, 0), Revision: 1,
	})); err != nil {
		t.Fatalf("seed Session %q: %v", sessionID, err)
	}
}

func seedWorkspaceMutationRun(t *testing.T, db *sql.DB, sessionID, runID string) {
	t.Helper()
	if err := NewRunStore(db).Restore(t.Context(), testsupport.MustRestoreRun(run.Snapshot{
		SessionID: sessionID, ID: runID, State: run.Completed,
		CreatedAt: time.Unix(1, 0), FinishedAt: time.Unix(2, 0), UpdatedAt: time.Unix(2, 0),
	})); err != nil {
		t.Fatalf("seed Run %q: %v", runID, err)
	}
}

// TestWorkspaceMutationLogRoundTrip: a recorded intent surfaces in ListPending
// and clears on Complete, leaving boot recovery only unfinished mutations.
func TestWorkspaceMutationLogRoundTrip(t *testing.T) {
	db, err := Open(t.Context(), ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := NewWorkspaceMutationStore(db)
	ctx := context.Background()
	seedWorkspaceMutationSession(t, db, "ses_1", "/repo")
	seedWorkspaceMutationRun(t, db, "ses_1", "run_1")
	seedWorkspaceMutationSession(t, db, "ses_2", "/repo2")
	seedWorkspaceMutationRun(t, db, "ses_2", "run_9")

	if recordErr := store.Record(ctx, WorkspaceMutationRecord{
		SessionID: "ses_1", CWD: "/repo", ToRunID: "run_1", RestoreHistory: true,
	}); recordErr != nil {
		t.Fatalf("record: %v", recordErr)
	}
	if recordErr := store.Record(ctx, WorkspaceMutationRecord{SessionID: "ses_2", CWD: "/repo2", ToRunID: "run_9"}); recordErr != nil {
		t.Fatalf("record 2: %v", recordErr)
	}

	pending, err := store.ListPending(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(pending) != 2 {
		t.Fatalf("pending = %d, want 2", len(pending))
	}
	if pending[0] != (WorkspaceMutationRecord{
		SessionID: "ses_1", CWD: "/repo", ToRunID: "run_1", RestoreHistory: true,
	}) {
		t.Fatalf("pending[0] = %+v, want the ses_1 intent verbatim", pending[0])
	}

	if err := store.Complete(ctx, "ses_1"); err != nil {
		t.Fatalf("complete: %v", err)
	}
	// Completing an already-cleared row is a no-op (boot recovery may re-complete).
	if err := store.Complete(ctx, "ses_1"); err != nil {
		t.Fatalf("re-complete: %v", err)
	}

	pending, _ = store.ListPending(ctx)
	if len(pending) != 1 || pending[0].SessionID != "ses_2" {
		t.Fatalf("pending after complete = %+v, want only ses_2", pending)
	}
}

// TestWorkspaceMutationReRecordReplaces: re-recording for the same session
// overwrites rather than duplicating (the mutation slot admits one per session).
func TestWorkspaceMutationReRecordReplaces(t *testing.T) {
	db, err := Open(t.Context(), ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := NewWorkspaceMutationStore(db)
	ctx := context.Background()
	seedWorkspaceMutationSession(t, db, "ses_1", "/a")
	seedWorkspaceMutationRun(t, db, "ses_1", "run_1")
	seedWorkspaceMutationRun(t, db, "ses_1", "run_2")

	if err := store.Record(ctx, WorkspaceMutationRecord{SessionID: "ses_1", CWD: "/a", ToRunID: "run_1"}); err != nil {
		t.Fatalf("record first intent: %v", err)
	}
	if err := store.Record(ctx, WorkspaceMutationRecord{SessionID: "ses_1", CWD: "/a", ToRunID: "run_2"}); err != nil {
		t.Fatalf("replace intent: %v", err)
	}

	pending, _ := store.ListPending(ctx)
	if len(pending) != 1 || pending[0].CWD != "/a" || pending[0].ToRunID != "run_2" {
		t.Fatalf("pending = %+v, want one ses_1 row with the latest intent", pending)
	}
}

func TestPendingWorkspaceMutationFencesItsRecoveryInputs(t *testing.T) {
	db, err := Open(t.Context(), ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := t.Context()
	seedWorkspaceMutationSession(t, db, "ses_owner", "/repo")
	seedWorkspaceMutationRun(t, db, "ses_owner", "run_target")
	seedWorkspaceMutationSession(t, db, "ses_sibling", "/repo")
	mutations := NewWorkspaceMutationStore(db)
	if err := mutations.Record(ctx, WorkspaceMutationRecord{
		SessionID: "ses_owner", CWD: "/repo", ToRunID: "run_target", RestoreHistory: true,
	}); err != nil {
		t.Fatalf("record mutation: %v", err)
	}

	runs := NewRunStore(db)
	for _, draft := range []run.Draft{
		{RunID: "run_owner_next", SessionID: "ses_owner", SegmentID: "seg_open", CreatedAt: time.Unix(3, 0)},
		{RunID: "run_sibling_next", SessionID: "ses_sibling", SegmentID: "seg_open", CreatedAt: time.Unix(3, 0)},
	} {
		draft = testsupport.RunDraft(draft)
		if err := runs.Admit(ctx, draft); !errors.Is(err, run.ErrSessionBusy) {
			t.Fatalf("admit %q during pending mutation = %v, want ErrSessionBusy", draft.RunID, err)
		}
	}
	if err := runs.Delete(ctx, "ses_owner", "run_target"); err == nil {
		t.Fatal("deleted rollback target while mutation was pending")
	}
	if err := NewSessionStore(db).Delete(ctx, "ses_owner"); err == nil {
		t.Fatal("deleted Session while its workspace mutation was pending")
	}

	if err := mutations.Complete(ctx, "ses_owner"); err != nil {
		t.Fatalf("complete mutation: %v", err)
	}
	if err := runs.Admit(ctx, testsupport.RunDraft(run.Draft{
		RunID: "run_owner_next", SessionID: "ses_owner", SegmentID: "seg_open", CreatedAt: time.Unix(3, 0),
	})); err != nil {
		t.Fatalf("admit after completion: %v", err)
	}
}
