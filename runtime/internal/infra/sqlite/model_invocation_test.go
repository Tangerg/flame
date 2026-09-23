package sqlite_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
	"github.com/Tangerg/flame/runtime/internal/infra/sqlite"
	"github.com/Tangerg/scope/core/chat"
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
	latencies := []*int64{nil, new(int64(0)), new(int64(124))}
	usages := []*chat.Usage{nil, {}, {InputTokens: 17, OutputTokens: 9, CacheReadInputTokens: new(int64(4)), CacheWriteInputTokens: new(int64(2)), ReasoningTokens: new(int64(3))}}
	for index, id := range []string{"call_a", "call_b", "call_c"} {
		if err := calls.StartModelInvocation(ctx, draft.SessionID, draft.RunID, draft.SegmentID, id, startedAt); err != nil {
			t.Fatal(err)
		}
		if err := calls.CompleteModelInvocation(ctx, draft.SessionID, draft.RunID, draft.SegmentID, id, startedAt, startedAt.Add(time.Second), latencies[index], usages[index]); err != nil {
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
	if page[0].FirstOutputLatencyMillis == nil || *page[0].FirstOutputLatencyMillis != 124 || page[1].FirstOutputLatencyMillis == nil || *page[1].FirstOutputLatencyMillis != 0 || second[0].FirstOutputLatencyMillis != nil {
		t.Fatalf("first output latency lost absent/zero/measured distinction: %+v %+v", page, second)
	}
	if page[0].Usage == nil || !accounting.ReportedUsageEqual(*page[0].Usage, *usages[2]) ||
		page[1].Usage == nil || !accounting.ReportedUsageEqual(*page[1].Usage, *usages[1]) ||
		second[0].Usage != nil {
		t.Fatalf("restored usage lost unknown/zero/per-call values: %+v %+v", page, second)
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

func TestModelInvocationUsageAdoptsExistingDatabaseWithoutInventingHistory(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "existing.sqlite")
	db, err := sqlite.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	draft := runDraft("run_existing", "ses_existing")
	if err := sqlite.NewRunStore(db).Admit(ctx, draft); err != nil {
		t.Fatal(err)
	}
	calls := sqlite.NewModelInvocationStore(db)
	if err := calls.StartModelInvocation(ctx, draft.SessionID, draft.RunID, draft.SegmentID, "call_old", draft.CreatedAt); err != nil {
		t.Fatal(err)
	}
	if err := calls.CompleteModelInvocation(ctx, draft.SessionID, draft.RunID, draft.SegmentID, "call_old", draft.CreatedAt, draft.CreatedAt, nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "ALTER TABLE model_invocations DROP COLUMN first_output_latency_millis"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "ALTER TABLE model_invocations DROP COLUMN usage"); err != nil {
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
	rows, err := calls.PageModelInvocations(ctx, draft.RunID, 0, "", 10)
	if err != nil || len(rows) != 1 || rows[0].Usage != nil || rows[0].FirstOutputLatencyMillis != nil || rows[0].State != "completed" {
		t.Fatalf("historical attempt = %+v, %v", rows, err)
	}
	if err := calls.StartModelInvocation(ctx, draft.SessionID, draft.RunID, draft.SegmentID, "call_new", draft.CreatedAt); err != nil {
		t.Fatal(err)
	}
	if err := calls.CompleteModelInvocation(ctx, draft.SessionID, draft.RunID, draft.SegmentID, "call_new", draft.CreatedAt, draft.CreatedAt, nil, &chat.Usage{InputTokens: 3}); err != nil {
		t.Fatal(err)
	}
}

func TestFailedModelInvocationRetainsMeasuredLatencyWithoutUsage(t *testing.T) {
	db, err := sqlite.Open(t.Context(), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	draft := runDraft("run_latency", "ses_latency")
	if err := sqlite.NewRunStore(db).Admit(t.Context(), draft); err != nil {
		t.Fatal(err)
	}
	calls := sqlite.NewModelInvocationStore(db)
	if err := calls.StartModelInvocation(t.Context(), draft.SessionID, draft.RunID, draft.SegmentID, "call_latency", draft.CreatedAt); err != nil {
		t.Fatal(err)
	}
	if err := calls.FailModelInvocation(t.Context(), draft.SessionID, draft.RunID, draft.SegmentID, "call_latency", draft.CreatedAt, draft.CreatedAt.Add(time.Second), new(int64(7))); err != nil {
		t.Fatal(err)
	}
	page, err := calls.PageModelInvocations(t.Context(), draft.RunID, 0, "", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(page) != 1 || page[0].State != "failed" || page[0].Usage != nil || page[0].FirstOutputLatencyMillis == nil || *page[0].FirstOutputLatencyMillis != 7 {
		t.Fatalf("failed call = %+v", page)
	}
}
