package persistence

import (
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
)

func TestInvocationAdmissionsRequireTheOriginalActiveSegment(t *testing.T) {
	db, err := openDatabase(t.Context(), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	draft := runDraft("run_admission", "ses_admission")
	store := newRunStore(db)
	if err := store.Admit(t.Context(), draft); err != nil {
		t.Fatal(err)
	}
	models := newModelInvocationStore(db)
	tools := newToolInvocationStore(db)
	refuse := func(segmentID string) {
		t.Helper()
		if err := models.StartModelInvocation(t.Context(), draft.SessionID, draft.RunID, segmentID, "model_admission", draft.CreatedAt, run.ModelConversation); err == nil {
			t.Fatal("model admission used an inactive Segment")
		}
		if err := tools.StartToolInvocation(t.Context(), draft.SessionID, draft.RunID, segmentID, "tool_admission", "item_admission", draft.CreatedAt, nil); err == nil {
			t.Fatal("Tool admission used an inactive Segment")
		}
		modelRows, err := models.PageModelInvocations(t.Context(), draft.RunID, 0, "", 10)
		if err != nil || len(modelRows) != 0 {
			t.Fatalf("refused model admissions = (%+v, %v)", modelRows, err)
		}
		toolRows, err := tools.ListSession(t.Context(), draft.SessionID)
		if err != nil || len(toolRows) != 0 {
			t.Fatalf("refused Tool admissions = (%+v, %v)", toolRows, err)
		}
	}
	refuse("segment_other")
	active, found, err := store.Run(t.Context(), draft.RunID)
	if err != nil || !found {
		t.Fatalf("admitted Run = (%v, %v)", found, err)
	}
	waiting, err := active.Suspend(draft.CreatedAt.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Suspend(t.Context(), waiting, draft.SegmentID, runs.CommitID{}); err != nil {
		t.Fatal(err)
	}
	refuse(draft.SegmentID)
}
