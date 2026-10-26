package persistence

import (
	"testing"
	"time"

	rundomain "github.com/Tangerg/flame/runtime/internal/domain/run"
)

func TestInvocationJournalsDistinguishEpochSettlementFromNoSettlement(t *testing.T) {
	db, err := openDatabase(t.Context(), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	draft := runDraft("run_time", "ses_time")
	startedAt, finishedAt := time.Unix(-1, 0).UTC(), time.Unix(0, 0).UTC()
	draft.CreatedAt = startedAt
	if err := newRunStore(db).Admit(t.Context(), draft); err != nil {
		t.Fatal(err)
	}
	models, tools := newModelInvocationStore(db), newToolInvocationStore(db)
	if err := models.StartModelInvocation(t.Context(), draft.SessionID, draft.RunID, draft.SegmentID, "call_model", startedAt, rundomain.ModelConversation); err != nil {
		t.Fatal(err)
	}
	if err := tools.StartToolInvocation(t.Context(), draft.SessionID, draft.RunID, draft.SegmentID, "call_tool", "item_tool", startedAt); err != nil {
		t.Fatal(err)
	}
	reader, err := NewModelInvocationReader(models)
	if err != nil {
		t.Fatal(err)
	}
	modelRows, err := reader.PageModelInvocations(t.Context(), draft.RunID, 0, "", 10)
	if err != nil || len(modelRows) != 1 || !modelRows[0].FinishedAt.IsZero() {
		t.Fatalf("open Model attempt = %+v, %v", modelRows, err)
	}
	toolRows, err := tools.ListSession(t.Context(), draft.SessionID)
	if err != nil || len(toolRows) != 1 || !toolRows[0].FinishedAt.IsZero() {
		t.Fatalf("open Tool attempt = %+v, %v", toolRows, err)
	}
	if err := models.CompleteModelInvocation(t.Context(), draft.SessionID, draft.RunID, draft.SegmentID, "call_model", startedAt, finishedAt, nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := tools.CompleteToolInvocation(t.Context(), draft.SessionID, draft.RunID, draft.SegmentID, "call_tool", "item_tool", startedAt, finishedAt); err != nil {
		t.Fatal(err)
	}
	modelRows, err = reader.PageModelInvocations(t.Context(), draft.RunID, 0, "", 10)
	if err != nil || len(modelRows) != 1 || !modelRows[0].FinishedAt.Equal(finishedAt) {
		t.Errorf("settled Model attempt = %+v, %v; want Unix epoch", modelRows, err)
	}
	toolRows, err = tools.ListSession(t.Context(), draft.SessionID)
	if err != nil || len(toolRows) != 1 || !toolRows[0].FinishedAt.Equal(finishedAt) {
		t.Errorf("settled Tool attempt = %+v, %v; want Unix epoch", toolRows, err)
	}
}
