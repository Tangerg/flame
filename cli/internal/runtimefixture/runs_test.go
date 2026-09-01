package runtimefixture

import (
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/flame/cli/internal/domain/agent"
	"github.com/Tangerg/flame/runtime/protocol"
)

func TestRunCatalogReadsFiltersAndPaginatesNewestFirst(t *testing.T) {
	runtime := New()
	runtime.Script = func(string) Script {
		return Script{Prelude: []Step{eventStep(time.Hour, agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}})}}
	}
	opened, err := runtime.StartRun(t.Context(), unlimitedStartRun("ses_demo_1", "active"))
	if err != nil {
		t.Fatal(err)
	}

	got, err := runtime.GetRun(t.Context(), opened.RunID)
	if err != nil || got.Status != protocol.RunStatusRunning {
		t.Fatalf("GetRun = %+v, %v", got, err)
	}
	pageSize, err := agent.NewPageSize(1)
	if err != nil {
		t.Fatal(err)
	}
	page, err := runtime.ListRuns(t.Context(), agent.RunQuery{SessionID: "ses_demo_1", PageSize: pageSize})
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != opened.RunID || page.NextCursor == "" {
		t.Fatalf("first page = %+v, %v", page, err)
	}
	next, err := runtime.ListRuns(t.Context(), agent.RunQuery{SessionID: "ses_demo_1", PageSize: pageSize, Cursor: page.NextCursor})
	if err != nil || len(next.Items) != 1 || next.Items[0].ID != "run_demo_history" || next.NextCursor != "" {
		t.Fatalf("second page = %+v, %v", next, err)
	}
	waiting, err := runtime.ListRuns(t.Context(), agent.RunQuery{
		PageSize: agent.DefaultPageSize(), Statuses: []protocol.RunStatus{protocol.RunStatusWaiting},
	})
	if err != nil || len(waiting.Items) != 0 {
		t.Fatalf("waiting page = %+v, %v", waiting, err)
	}
	if _, err := runtime.CancelRun(t.Context(), agent.CancelRun{RunID: opened.RunID}); err != nil {
		t.Fatal(err)
	}
}

func TestRunCatalogRetainsLatestProgressFootprint(t *testing.T) {
	runtime := New()
	contextTokens := int64(12_345)
	runtime.Script = func(string) Script {
		return Script{Prelude: []Step{
			eventStep(0, agent.RunProgress{ContextTokens: &contextTokens, Usage: &agent.Usage{InputTokens: 40}}),
			eventStep(time.Hour, agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}),
		}}
	}
	opened, err := runtime.StartRun(t.Context(), unlimitedStartRun("ses_demo_1", "progress"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = runtime.CancelRun(t.Context(), agent.CancelRun{RunID: opened.RunID})
	}()
	for event, streamErr := range opened.Events {
		if streamErr != nil {
			t.Fatal(streamErr)
		}
		if _, progress := event.Event.(agent.RunProgress); progress {
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
			eventStep(0, agent.RunProgress{ContextTokens: &contextTokens}),
			eventStep(0, agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}),
		}}
	}
	opened, err := runtime.StartRun(t.Context(), unlimitedStartRun("ses_demo_1", "progress"))
	if err != nil {
		t.Fatal(err)
	}
	var finished agent.RunFinished
	for event, streamErr := range opened.Events {
		if streamErr != nil {
			t.Fatal(streamErr)
		}
		if boundary, ok := event.Event.(agent.RunFinished); ok {
			finished = boundary
		}
	}
	if finished.ContextTokens != contextTokens {
		t.Fatalf("finished context tokens = %d, want %d", finished.ContextTokens, contextTokens)
	}
	got, err := runtime.GetRun(t.Context(), opened.RunID)
	if err != nil || got.ContextTokens != contextTokens {
		t.Fatalf("GetRun after finish = %+v, %v", got, err)
	}
}

func TestRunCatalogDoesNotRetainDeletedSessionRuns(t *testing.T) {
	runtime := New()
	if err := runtime.DeleteSession(t.Context(), agent.DeleteSession{SessionID: "ses_demo_1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.GetRun(t.Context(), "run_demo_history"); !errors.Is(err, agent.ErrRunNotFound) {
		t.Fatalf("GetRun after session deletion = %v", err)
	}
	page, err := runtime.ListRuns(t.Context(), agent.RunQuery{
		SessionID: "ses_demo_1", PageSize: agent.DefaultPageSize(),
	})
	if err != nil || len(page.Items) != 0 {
		t.Fatalf("ListRuns after session deletion = %+v, %v", page, err)
	}
}
