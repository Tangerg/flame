package persistence

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func seedWorkspaceMutationSession(t *testing.T, db *sql.DB, sessionID, cwd string) {
	t.Helper()
	if err := newSessionStore(db).Insert(t.Context(), testsupport.MustRestoreSession(session.Snapshot{
		ID: sessionID, Workspace: testsupport.MustWorkspace(cwd),
		CreatedAt: time.Unix(1, 0), UpdatedAt: time.Unix(1, 0), Revision: 1,
	})); err != nil {
		t.Fatalf("seed Session %q: %v", sessionID, err)
	}
}

func seedWorkspaceMutationRun(t *testing.T, db *sql.DB, sessionID, runID string) {
	t.Helper()
	if err := newRunStore(db).Restore(t.Context(), testsupport.MustRestoreRun(run.Snapshot{
		SessionID: sessionID, ID: runID, State: run.Completed,
		CreatedAt: time.Unix(1, 0), FinishedAt: time.Unix(2, 0), UpdatedAt: time.Unix(2, 0),
	})); err != nil {
		t.Fatalf("seed Run %q: %v", runID, err)
	}
}

// TestWorkspaceMutationLogRoundTrip: a recorded intent surfaces in ListPending
// and clears on Complete, leaving boot recovery only unfinished mutations.
func TestWorkspaceMutationLogRoundTrip(t *testing.T) {
	db, err := openDatabase(t.Context(), ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := newWorkspaceMutationStore(db)
	ctx := context.Background()
	seedWorkspaceMutationSession(t, db, "ses_1", "/repo")
	seedWorkspaceMutationRun(t, db, "ses_1", "run_1")
	seedWorkspaceMutationSession(t, db, "ses_2", "/repo2")
	seedWorkspaceMutationRun(t, db, "ses_2", "run_9")

	if _, recordErr := store.recordMutation(ctx, workspaceMutationRecord{
		SessionID: "ses_1", CWD: "/repo", ToRunID: "run_1", RestoreHistory: true,
	}); recordErr != nil {
		t.Fatalf("record: %v", recordErr)
	}
	if _, recordErr := store.recordMutation(ctx, workspaceMutationRecord{SessionID: "ses_2", CWD: "/repo2", ToRunID: "run_9"}); recordErr != nil {
		t.Fatalf("record 2: %v", recordErr)
	}

	pending, err := store.pendingMutations(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(pending) != 2 {
		t.Fatalf("pending = %d, want 2", len(pending))
	}
	if pending[0] != (workspaceMutationRecord{
		SessionID: "ses_1", CWD: "/repo", ToRunID: "run_1", RestoreHistory: true,
	}) {
		t.Fatalf("pending[0] = %+v, want the ses_1 intent verbatim", pending[0])
	}

	completed := workspaceMutationRecord{
		SessionID: "ses_1", CWD: "/repo", ToRunID: "run_1", RestoreHistory: true,
	}
	if err := store.completeMutation(ctx, completed); err != nil {
		t.Fatalf("complete: %v", err)
	}
	// Completing an already-cleared row is a no-op (boot recovery may re-complete).
	if err := store.completeMutation(ctx, completed); err != nil {
		t.Fatalf("re-complete: %v", err)
	}

	pending, _ = store.pendingMutations(ctx)
	if len(pending) != 1 || pending[0].SessionID != "ses_2" {
		t.Fatalf("pending after complete = %+v, want only ses_2", pending)
	}
}

// TestUnfinishedWorkspaceMutationKeepsRecoveryOwnership: an intent left pending
// by an incomplete reset cannot be displaced or cleared by another rollback, so
// boot recovery still has the evidence that tree was partially reset.
func TestUnfinishedWorkspaceMutationKeepsRecoveryOwnership(t *testing.T) {
	db, err := openDatabase(t.Context(), ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := newWorkspaceMutationStore(db)
	ctx := context.Background()
	seedWorkspaceMutationSession(t, db, "ses_1", "/a")
	seedWorkspaceMutationRun(t, db, "ses_1", "run_1")
	seedWorkspaceMutationRun(t, db, "ses_1", "run_2")
	seedWorkspaceMutationSession(t, db, "ses_sibling", "/a")
	seedWorkspaceMutationRun(t, db, "ses_sibling", "run_sibling")

	unfinished := workspaceMutationRecord{SessionID: "ses_1", CWD: "/a", ToRunID: "run_1"}
	if created, err := store.recordMutation(ctx, unfinished); err != nil || !created {
		t.Fatalf("record first intent: created = %t, error = %v", created, err)
	}
	if created, err := store.recordMutation(ctx, unfinished); err != nil || created {
		t.Fatalf("re-record own intent: created = %t, error = %v", created, err)
	}

	displacing := []workspaceMutationRecord{
		{SessionID: "ses_1", CWD: "/a", ToRunID: "run_2"},
		{SessionID: "ses_1", CWD: "/a", ToRunID: "run_1", RestoreHistory: true},
		{SessionID: "ses_sibling", CWD: "/a", ToRunID: "run_sibling"},
	}
	for _, m := range displacing {
		if _, err := store.recordMutation(ctx, m); !errors.Is(err, errWorkspaceMutationPending) {
			t.Fatalf("record %+v = %v, want errWorkspaceMutationPending", m, err)
		}
	}
	for _, m := range displacing {
		if err := store.completeMutation(ctx, m); err != nil {
			t.Fatalf("complete %+v: %v", m, err)
		}
	}

	pending, _ := store.pendingMutations(ctx)
	if len(pending) != 1 || pending[0] != unfinished {
		t.Fatalf("pending = %+v, want only the unfinished ses_1 intent", pending)
	}
	if err := store.completeMutation(ctx, unfinished); err != nil {
		t.Fatalf("complete own intent: %v", err)
	}
	if pending, _ = store.pendingMutations(ctx); len(pending) != 0 {
		t.Fatalf("pending after owner completed = %+v, want none", pending)
	}
}

func TestPendingWorkspaceMutationFencesItsRecoveryInputs(t *testing.T) {
	db, err := openDatabase(t.Context(), ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := t.Context()
	seedWorkspaceMutationSession(t, db, "ses_owner", "/repo")
	seedWorkspaceMutationRun(t, db, "ses_owner", "run_target")
	seedWorkspaceMutationSession(t, db, "ses_sibling", "/repo")
	mutations := newWorkspaceMutationStore(db)
	if _, err := mutations.recordMutation(ctx, workspaceMutationRecord{
		SessionID: "ses_owner", CWD: "/repo", ToRunID: "run_target", RestoreHistory: true,
	}); err != nil {
		t.Fatalf("record mutation: %v", err)
	}

	runs := newRunStore(db)
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
	if err := newSessionStore(db).Delete(ctx, "ses_owner"); err == nil {
		t.Fatal("deleted Session while its workspace mutation was pending")
	}

	if err := mutations.completeMutation(ctx, workspaceMutationRecord{
		SessionID: "ses_owner", CWD: "/repo", ToRunID: "run_target", RestoreHistory: true,
	}); err != nil {
		t.Fatalf("complete mutation: %v", err)
	}
	if err := runs.Admit(ctx, testsupport.RunDraft(run.Draft{
		RunID: "run_owner_next", SessionID: "ses_owner", SegmentID: "seg_open", CreatedAt: time.Unix(3, 0),
	})); err != nil {
		t.Fatalf("admit after completion: %v", err)
	}
}

func TestRetainedWorkspaceMutationFencesOverlappingTreesAfterReopen(t *testing.T) {
	for _, pair := range [][2]string{{"/repo", "/repo/a"}, {"/repo/a", "/repo"}, {"/repo/a", "/repo/ab"}} {
		t.Run(pair[0]+"/"+pair[1], func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "runtime.db")
			db, err := openDatabase(t.Context(), path)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Close() })
			seedWorkspaceMutationSession(t, db, "ses_owner", pair[0])
			seedWorkspaceMutationRun(t, db, "ses_owner", "run_boundary")
			seedWorkspaceMutationSession(t, db, "ses_other", pair[1])
			seedWorkspaceMutationRun(t, db, "ses_other", "run_other_boundary")
			intent := workspaceMutationRecord{SessionID: "ses_owner", CWD: pair[0], ToRunID: "run_boundary"}
			if _, err := newWorkspaceMutationStore(db).recordMutation(t.Context(), intent); err != nil {
				t.Fatal(err)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			reopened, err := openDatabase(t.Context(), path)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = reopened.Close() })
			store := newWorkspaceMutationStore(reopened)
			_, err = store.recordMutation(t.Context(), workspaceMutationRecord{SessionID: "ses_other", CWD: pair[1], ToRunID: "run_other_boundary"})
			disjoint := pair[1] == "/repo/ab"
			if disjoint {
				if err != nil {
					t.Fatalf("disjoint mutation refused: %v", err)
				}
				if err := store.completeMutation(t.Context(), workspaceMutationRecord{SessionID: "ses_other", CWD: pair[1], ToRunID: "run_other_boundary"}); err != nil {
					t.Fatal(err)
				}
			} else if !errors.Is(err, errWorkspaceMutationPending) {
				t.Fatalf("overlapping mutation = %v, want retained ownership refusal", err)
			}
			draft := testsupport.RunDraft(run.Draft{RunID: "run_next", SessionID: "ses_other", SegmentID: "seg_next", CreatedAt: time.Unix(3, 0)})
			err = newRunStore(reopened).Admit(t.Context(), draft)
			if disjoint {
				if err != nil {
					t.Fatalf("disjoint Run admission = %v", err)
				}
			} else {
				if !errors.Is(err, run.ErrSessionBusy) {
					t.Fatalf("overlapping Run admission = %v, want busy", err)
				}
				if err := store.completeMutation(t.Context(), intent); err != nil {
					t.Fatal(err)
				}
				if err := newRunStore(reopened).Admit(t.Context(), draft); err != nil {
					t.Fatalf("admission after completion = %v", err)
				}
			}
		})
	}
}

func TestPendingWorkspaceMutationAlsoFencesResume(t *testing.T) {
	for _, pair := range [][2]string{{"/repo", "/repo/a"}, {"/repo/a", "/repo"}} {
		t.Run(pair[0]+"/"+pair[1], func(t *testing.T) {
			runs, _ := newRunStores(t)
			seedWorkspaceMutationSession(t, runs.db, "ses_owner", pair[0])
			seedWorkspaceMutationRun(t, runs.db, "ses_owner", "run_boundary")
			seedWorkspaceMutationSession(t, runs.db, "ses_waiting", pair[1])
			draft := testsupport.RunDraft(run.Draft{RunID: "run_waiting", SessionID: "ses_waiting", SegmentID: "seg_initial", CreatedAt: time.Unix(3, 0)})
			if err := runs.Admit(t.Context(), draft); err != nil {
				t.Fatal(err)
			}
			parked := parkedRunFromDraft(draft)
			if err := suspendRun(t.Context(), runs, parked, draft.SegmentID); err != nil {
				t.Fatal(err)
			}
			intent := workspaceMutationRecord{SessionID: "ses_owner", CWD: pair[0], ToRunID: "run_boundary"}
			mutations := newWorkspaceMutationStore(runs.db)
			if _, err := mutations.recordMutation(t.Context(), intent); err != nil {
				t.Fatal(err)
			}
			if err := resumeParkedRun(t.Context(), runs, parked, "seg_next", parked.UpdatedAt()); !errors.Is(err, run.ErrSessionBusy) {
				t.Fatalf("resume across pending mutation = %v, want busy", err)
			}
			assertStoredOpenRun(t, runs, parked)
			if err := mutations.completeMutation(t.Context(), intent); err != nil {
				t.Fatal(err)
			}
			if err := resumeParkedRun(t.Context(), runs, parked, "seg_next", parked.UpdatedAt()); err != nil {
				t.Fatalf("resume after mutation completion = %v", err)
			}
		})
	}
}

func TestRecoveryRefusesOverlappingRetainedMutationOwnership(t *testing.T) {
	runs, _ := newRunStores(t)
	for _, scope := range []struct{ session, run, cwd string }{{"ses_parent", "run_parent", "/repo"}, {"ses_child", "run_child", "/repo/a"}} {
		seedWorkspaceMutationSession(t, runs.db, scope.session, scope.cwd)
		seedWorkspaceMutationRun(t, runs.db, scope.session, scope.run)
	}
	store := newWorkspaceMutationStore(runs.db)
	if _, err := store.recordMutation(t.Context(), workspaceMutationRecord{SessionID: "ses_parent", CWD: "/repo", ToRunID: "run_parent"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.pendingMutations(t.Context()); err != nil {
		t.Fatalf("valid retained ownership refused: %v", err)
	}
	if _, err := runs.db.ExecContext(t.Context(), `INSERT INTO pending_workspace_mutations(session_id, cwd, to_run_id, restore_history) VALUES ('ses_child', '/repo/a', 'run_child', 0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.pendingMutations(t.Context()); err == nil {
		t.Fatal("recovery accepted two owners of overlapping file ranges")
	}
}
