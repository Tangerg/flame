package agent

import (
	"strings"
	"testing"
)

func TestRunQueryRejectsInvalidFilters(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name  string
		query RunQuery
		want  string
	}{
		{name: "negative page size", query: RunQuery{PageSize: PageSize{kind: explicitPageSize, rows: -1}}, want: "page size"},
		{name: "unknown status", query: RunQuery{PageSize: DefaultPageSize(), Statuses: []RunStatus{"paused"}}, want: "paused"},
		{name: "duplicate status", query: RunQuery{PageSize: DefaultPageSize(), Statuses: []RunStatus{RunStatusRunning, RunStatusRunning}}, want: "repeated"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if err := test.query.Validate(); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate() = %v, want error containing %q", err, test.want)
			}
		})
	}
}

func TestRunPageValidatesItemsIndependentlyOfTreePageBoundaries(t *testing.T) {
	t.Parallel()
	lineage := testChildRunLineage(t, "run_child", "item_spawn", "run_parent", "run_root")
	page := RunPage{Items: []Run{{
		ID: "run_child", SessionID: "ses_1",
		Lineage: lineage,
		Status:  RunStatusWaiting, Limits: UnlimitedRunLimits(),
	}}}
	if err := page.Validate(); err != nil {
		t.Fatalf("Validate() = %v", err)
	}
	page.Items = append(page.Items, page.Items[0])
	if err := page.Validate(); err == nil || !strings.Contains(err.Error(), "repeats id") {
		t.Fatalf("duplicate Validate() = %v", err)
	}
}

func TestRunCancellationClosesRootAndChildResults(t *testing.T) {
	t.Parallel()
	rootCanceled := Run{
		ID: "run_root", SessionID: "ses_1", Status: RunStatusFinished,
		Lineage: RootRunLineage(),
		Limits:  UnlimitedRunLimits(), Outcome: Outcome{Status: OutcomeCanceled},
	}
	if err := (RunCancellation{Canceled: rootCanceled, Root: rootCanceled}).Validate(); err != nil {
		t.Fatalf("root cancellation: %v", err)
	}

	childCanceled := Run{
		ID: "run_child", SessionID: "ses_1",
		Lineage: testChildRunLineage(t, "run_child", "item_spawn", "run_root", "run_root"),
		Status:  RunStatusFinished, Limits: UnlimitedRunLimits(), Outcome: Outcome{Status: OutcomeCanceled},
	}
	rootWaiting := testRootRun(Run{ID: "run_root", SessionID: "ses_1", Status: RunStatusWaiting})
	if err := (RunCancellation{Canceled: childCanceled, Root: rootWaiting}).Validate(); err != nil {
		t.Fatalf("child cancellation: %v", err)
	}

	wrongRoot := rootWaiting
	wrongRoot.ID = "run_other"
	if err := (RunCancellation{Canceled: childCanceled, Root: wrongRoot}).Validate(); err == nil || !strings.Contains(err.Error(), "does not belong") {
		t.Fatalf("wrong-root cancellation = %v", err)
	}
	if err := (RunCancellation{Canceled: rootCanceled, Root: rootCanceled}).ValidateTarget("run_other"); err == nil || !strings.Contains(err.Error(), "want") {
		t.Fatalf("wrong-target cancellation = %v", err)
	}
}
