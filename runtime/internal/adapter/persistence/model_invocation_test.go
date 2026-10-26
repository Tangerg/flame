package persistence

import (
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"

	"github.com/Tangerg/scope/core/chat"
)

func TestFailedModelKnownResponseSurvivesReopenWithoutPromotingOutcome(t *testing.T) {
	path := filepath.Join(t.TempDir(), "known-response.db")
	db, err := openDatabase(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	draft := runDraft("run_known", "ses_known")
	if err := NewRunStore(db).Admit(t.Context(), draft); err != nil {
		t.Fatal(err)
	}
	store := NewModelInvocationStore(db)
	message := chat.NewAssistantMessage(chat.NewTextPart("known answer"), chat.NewReasoningPart("basis", nil))
	response := &chat.Response{Output: &chat.Output{Message: &message, FinishReason: chat.FinishReasonStop}, Metadata: &chat.ResponseMetadata{Model: "served-model", Usage: &chat.Usage{InputTokens: 12, OutputTokens: 3, CacheReadInputTokens: new(int64(0))}}}
	if err := store.StartModelInvocation(t.Context(), draft.SessionID, draft.RunID, draft.SegmentID, "call_known", draft.CreatedAt, run.ModelConversation); err != nil {
		t.Fatal(err)
	}
	if err := store.FailModelInvocation(t.Context(), draft.SessionID, draft.RunID, draft.SegmentID, "call_known", draft.CreatedAt, draft.CreatedAt.Add(time.Second), nil, response.Metadata.Usage, response); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = openDatabase(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := NewModelInvocationStore(db).PageModelInvocations(t.Context(), draft.RunID, 0, "", 10)
	if err != nil || len(rows) != 1 || rows[0].State != "failed" || !reflect.DeepEqual(rows[0].KnownResponse, response) || !reflect.DeepEqual(rows[0].Usage, response.Metadata.Usage) {
		t.Fatalf("retained inner evidence=%+v %v", rows, err)
	}
}

func TestModelInvocationHistorySurvivesRestartAndPagesByStableIdentity(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "trajectory.sqlite")
	db, err := openDatabase(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	runs := NewRunStore(db)
	draft := runDraft("run_trajectory", "ses_trajectory")
	if err := runs.Admit(ctx, draft); err != nil {
		t.Fatal(err)
	}
	calls := NewModelInvocationStore(db)
	startedAt := draft.CreatedAt.Add(time.Second)
	latencies := []*int64{nil, new(int64(0)), new(int64(124))}
	usages := []*chat.Usage{nil, {}, {InputTokens: 17, OutputTokens: 9, CacheReadInputTokens: new(int64(4)), CacheWriteInputTokens: new(int64(2)), ReasoningTokens: new(int64(3))}}
	for index, id := range []string{"call_a", "call_b", "call_c"} {
		if err := calls.StartModelInvocation(ctx, draft.SessionID, draft.RunID, draft.SegmentID, id, startedAt, run.ModelConversation); err != nil {
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
	db, err = openDatabase(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	calls = NewModelInvocationStore(db)
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
	if err := NewRunStore(db).Delete(ctx, draft.SessionID, draft.RunID); err != nil {
		t.Fatal(err)
	}
	page, err = calls.PageModelInvocations(ctx, draft.RunID, 0, "", 2)
	if err != nil || len(page) != 0 {
		t.Fatalf("deleted Run page = %+v, %v", page, err)
	}
}

func TestRunCannotEraseAnUnsettledModelAttempt(t *testing.T) {
	db, err := openDatabase(t.Context(), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	runs := NewRunStore(db)
	draft := runDraft("run_pending", "ses_pending")
	if err := runs.Admit(t.Context(), draft); err != nil {
		t.Fatal(err)
	}
	calls := NewModelInvocationStore(db)
	if err := calls.StartModelInvocation(t.Context(), draft.SessionID, draft.RunID, draft.SegmentID, "call_pending", draft.CreatedAt, run.ModelConversation); err != nil {
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

func TestFailedModelInvocationRetainsMeasuredLatencyWithoutUsage(t *testing.T) {
	db, err := openDatabase(t.Context(), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	draft := runDraft("run_latency", "ses_latency")
	if err := NewRunStore(db).Admit(t.Context(), draft); err != nil {
		t.Fatal(err)
	}
	calls := NewModelInvocationStore(db)
	if err := calls.StartModelInvocation(t.Context(), draft.SessionID, draft.RunID, draft.SegmentID, "call_latency", draft.CreatedAt, run.ModelConversation); err != nil {
		t.Fatal(err)
	}
	if err := calls.FailModelInvocation(t.Context(), draft.SessionID, draft.RunID, draft.SegmentID, "call_latency", draft.CreatedAt, draft.CreatedAt.Add(time.Second), new(int64(7)), nil, nil); err != nil {
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
