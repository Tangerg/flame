package sqlite_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/infra/sqlite"
)

func TestModelInvocationHistorySurvivesRestartAndPagesByStableIdentity(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "trajectory.sqlite")
	db, err := sqlite.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	runs := sqlite.NewRunStore(db)
	draft := runDraft("run_trajectory", "ses_trajectory")
	if err := runs.Admit(ctx, draft); err != nil {
		t.Fatal(err)
	}
	calls := sqlite.NewModelInvocationStore(db)
	startedAt := draft.CreatedAt.Add(time.Second)
	for _, id := range []string{"call_a", "call_b", "call_c"} {
		if err := calls.StartModelInvocation(ctx, draft.SessionID, draft.RunID, draft.SegmentID, id, startedAt); err != nil {
			t.Fatal(err)
		}
		if err := calls.CompleteModelInvocation(ctx, draft.SessionID, draft.RunID, draft.SegmentID, id, startedAt, startedAt.Add(time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	if err := runs.Terminalize(ctx, storedRunReplacement(t, ctx, runs, finishedRunFromDraft(draft, run.OutcomeCompleted))); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = sqlite.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	calls = sqlite.NewModelInvocationStore(db)
	page, err := calls.PageModelInvocations(ctx, draft.RunID, 0, "", 2)
	if err != nil || len(page) != 2 || page[0].CallID != "call_c" || page[1].CallID != "call_b" {
		t.Fatalf("first page = %+v, %v", page, err)
	}
	second, err := calls.PageModelInvocations(ctx, draft.RunID, page[1].StartedAt.UnixNano(), page[1].CallID, 2)
	if err != nil || len(second) != 1 || second[0].CallID != "call_a" || second[0].State != "completed" {
		t.Fatalf("second page = %+v, %v", second, err)
	}
	other, err := calls.PageModelInvocations(ctx, "run_other", 0, "", 2)
	if err != nil || len(other) != 0 {
		t.Fatalf("other Run page = %+v, %v", other, err)
	}
	if err := sqlite.NewRunStore(db).Delete(ctx, draft.SessionID, draft.RunID); err != nil {
		t.Fatal(err)
	}
	page, err = calls.PageModelInvocations(ctx, draft.RunID, 0, "", 2)
	if err != nil || len(page) != 0 {
		t.Fatalf("deleted Run page = %+v, %v", page, err)
	}
}

func TestRunCannotEraseAnUnsettledModelAttempt(t *testing.T) {
	db, err := sqlite.Open(t.Context(), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	runs := sqlite.NewRunStore(db)
	draft := runDraft("run_pending", "ses_pending")
	if err := runs.Admit(t.Context(), draft); err != nil {
		t.Fatal(err)
	}
	calls := sqlite.NewModelInvocationStore(db)
	if err := calls.StartModelInvocation(t.Context(), draft.SessionID, draft.RunID, draft.SegmentID, "call_pending", draft.CreatedAt); err != nil {
		t.Fatal(err)
	}
	replacement := storedRunReplacement(t, t.Context(), runs, finishedRunFromDraft(draft, run.OutcomeCompleted))
	if err := runs.Terminalize(t.Context(), replacement); err == nil {
		t.Fatal("terminalized Run while provider outcome remains undecided")
	}
	rows, err := calls.PageModelInvocations(t.Context(), draft.RunID, 0, "", 2)
	if err != nil || len(rows) != 1 || rows[0].State != "started" {
		t.Fatalf("pending attempt = %+v, %v", rows, err)
	}
}
