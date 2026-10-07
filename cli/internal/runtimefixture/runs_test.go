package runtimefixture

import (
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/flame/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/runtime/protocol"
)

func TestRunCatalogReadsFiltersAndPaginatesNewestFirst(t *testing.T) {
	runtime := New()
	runtime.Script = func(string) Script {
		return Script{Prelude: []Step{finishStep(time.Hour, Finish{Outcome: conversation.Outcome{Status: protocol.OutcomeCompleted}})}}
	}
	opened, err := runtime.StartRun(t.Context(), testStartRun("ses_demo_1", "active"))
	if err != nil {
		t.Fatal(err)
	}

	got, err := runtime.GetRun(t.Context(), opened.RunID)
	if err != nil || got.Status != protocol.RunStatusRunning {
		t.Fatalf("GetRun = %+v, %v", got, err)
	}
	pageSize, err := conversation.NewPageSize(1)
	if err != nil {
		t.Fatal(err)
	}
	page, err := runtime.ListRuns(t.Context(), conversation.RunQuery{SessionID: "ses_demo_1", PageSize: pageSize})
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != opened.RunID || page.NextCursor == "" {
		t.Fatalf("first page = %+v, %v", page, err)
	}
	next, err := runtime.ListRuns(t.Context(), conversation.RunQuery{SessionID: "ses_demo_1", PageSize: pageSize, Cursor: page.NextCursor})
	if err != nil || len(next.Items) != 1 || next.Items[0].ID != "run_demo_history" || next.NextCursor != "" {
		t.Fatalf("second page = %+v, %v", next, err)
	}
	waiting, err := runtime.ListRuns(t.Context(), conversation.RunQuery{
		PageSize: conversation.DefaultPageSize(), Statuses: []protocol.RunStatus{protocol.RunStatusWaiting},
	})
	if err != nil || len(waiting.Items) != 0 {
		t.Fatalf("waiting page = %+v, %v", waiting, err)
	}
	if _, err := runtime.CancelRun(t.Context(), conversation.CancelRun{RunID: opened.RunID}); err != nil {
		t.Fatal(err)
	}
}

func TestRunCatalogRetainsLatestProgressFootprint(t *testing.T) {
	runtime := New()
	contextTokens := int64(12_345)
	runtime.Script = func(string) Script {
		return Script{Prelude: []Step{
			eventStep(0, conversation.RunProgress{ContextTokens: &contextTokens, Usage: &conversation.Usage{InputTokens: 40}}),
			finishStep(time.Hour, Finish{Outcome: conversation.Outcome{Status: protocol.OutcomeCompleted}}),
		}}
	}
	opened, err := runtime.StartRun(t.Context(), testStartRun("ses_demo_1", "progress"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = runtime.CancelRun(t.Context(), conversation.CancelRun{RunID: opened.RunID})
	}()
	for event, streamErr := range opened.Events {
		if streamErr != nil {
			t.Fatal(streamErr)
		}
		if _, progress := event.Event.(conversation.RunProgress); progress {
			break
		}
	}
	got, err := runtime.GetRun(t.Context(), opened.RunID)
	if err != nil || got.ContextTokens != contextTokens || got.Usage.InputTokens != 40 {
		t.Fatalf("GetRun after progress = %+v, %v", got, err)
	}
}

func TestRunStreamFinishesWithLatestProgressFootprint(t *testing.T) {
	runtime := New()
	contextTokens := int64(12_345)
	runtime.Script = func(string) Script {
		return Script{Prelude: []Step{
			eventStep(0, conversation.RunProgress{ContextTokens: &contextTokens}),
			finishStep(0, Finish{Outcome: conversation.Outcome{Status: protocol.OutcomeCompleted}}),
		}}
	}
	opened, err := runtime.StartRun(t.Context(), testStartRun("ses_demo_1", "progress"))
	if err != nil {
		t.Fatal(err)
	}
	var finished conversation.SegmentFinished
	for event, streamErr := range opened.Events {
		if streamErr != nil {
			t.Fatal(streamErr)
		}
		if boundary, ok := event.Event.(conversation.SegmentFinished); ok {
			finished = boundary
		}
	}
	if finished.Run.Status != protocol.RunStatusFinished || finished.Run.ContextTokens != contextTokens {
		t.Fatalf("finished run = %+v, want context tokens %d", finished.Run, contextTokens)
	}
	got, err := runtime.GetRun(t.Context(), opened.RunID)
	if err != nil || got.ContextTokens != contextTokens {
		t.Fatalf("GetRun after finish = %+v, %v", got, err)
	}
}

func TestRunCatalogDoesNotRetainDeletedSessionRuns(t *testing.T) {
	runtime := New()
	if err := runtime.DeleteSession(t.Context(), conversation.DeleteSession{SessionID: "ses_demo_1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.GetRun(t.Context(), "run_demo_history"); !errors.Is(err, conversation.ErrRunNotFound) {
		t.Fatalf("GetRun after session deletion = %v", err)
	}
	page, err := runtime.ListRuns(t.Context(), conversation.RunQuery{
		SessionID: "ses_demo_1", PageSize: conversation.DefaultPageSize(),
	})
	if err != nil || len(page.Items) != 0 {
		t.Fatalf("ListRuns after session deletion = %+v, %v", page, err)
	}
}
