package run

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
	"github.com/Tangerg/flame/runtime/internal/domain/run/interrupt"
	runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"
)

func TestRunAdmissionRejectsNonCanonicalOrUnboundedResourceIdentity(t *testing.T) {
	valid := Draft{RunID: "run_1", SessionID: "session_1", SegmentID: "segment_1", CreatedAt: time.Unix(1, 0)}
	for name, mutate := range map[string]func(*Draft){
		"run control":       func(draft *Draft) { draft.RunID = "run_\n1" },
		"session control":   func(draft *Draft) { draft.SessionID = "session_\t1" },
		"segment oversized": func(draft *Draft) { draft.SegmentID = strings.Repeat("s", runtimeidentity.MaximumResourceCharacters+1) },
	} {
		t.Run(name, func(t *testing.T) {
			draft := valid
			mutate(&draft)
			if _, err := Admit(draft); err == nil {
				t.Fatalf("invalid draft was accepted: %+v", draft)
			}
		})
	}
}

func TestRunLifecyclePreservesAdmissionFactsAndAdvancesMetrics(t *testing.T) {
	createdAt := time.Unix(1, 0).UTC()
	capabilities := Capabilities{ChildRuns: true, InterruptKinds: []interrupt.Kind{interrupt.Question}}
	value, err := Admit(Draft{
		RunID: "run_1", SessionID: "session_1", SegmentID: "segment_1",
		Capabilities: capabilities, CreatedAt: createdAt,
	})
	if err != nil {
		t.Fatalf("Admit: %v", err)
	}
	cost := 0.25
	metrics, err := NewMetrics(&accounting.Usage{
		Total: accounting.Totals{InputTokens: 3, OutputTokens: 2, CostUSD: &cost},
	}, 1, 2*time.Second)
	if err != nil {
		t.Fatalf("NewMetrics: %v", err)
	}
	updatedAt := createdAt.Add(3 * time.Second)
	value, err = value.AdvanceProgress(metrics, 0, updatedAt)
	if err != nil {
		t.Fatalf("AdvanceMetrics: %v", err)
	}
	value, err = value.Suspend(updatedAt)
	if err != nil {
		t.Fatalf("Suspend: %v", err)
	}
	if value.State() != Waiting || value.ActiveSegmentID() != "" || !value.Metrics().Equal(metrics) {
		t.Fatalf("suspended Run = %+v", value.Snapshot())
	}
	resumedAt := updatedAt.Add(time.Second)
	value, err = value.Resume("segment_2", resumedAt)
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if value.State() != Running || value.ActiveSegmentID() != "segment_2" ||
		!value.Capabilities().Equal(capabilities) || !value.CreatedAt().Equal(createdAt) {
		t.Fatalf("resumed Run = %+v", value.Snapshot())
	}
}

func TestRunProgressPreservesLatestPromptFootprintAcrossLifecycle(t *testing.T) {
	createdAt := time.Unix(1, 0).UTC()
	value, err := Admit(Draft{
		RunID: "run_1", SessionID: "session_1", SegmentID: "segment_1", CreatedAt: createdAt,
	})
	if err != nil {
		t.Fatalf("Admit: %v", err)
	}
	metrics, err := NewMetrics(nil, 1, time.Second)
	if err != nil {
		t.Fatalf("NewMetrics: %v", err)
	}
	value, err = value.AdvanceProgress(metrics, 198_000, createdAt.Add(time.Second))
	if err != nil {
		t.Fatalf("AdvanceProgress: %v", err)
	}
	if got := value.ContextTokens(); got != 198_000 {
		t.Fatalf("ContextTokens = %d, want 198000", got)
	}

	// Prompt footprint is a point-in-time fact, not cumulative accounting: a
	// compaction may legitimately make it smaller while Metrics stay monotonic.
	value, err = value.AdvanceProgress(metrics, 87_900, createdAt.Add(2*time.Second))
	if err != nil {
		t.Fatalf("AdvanceProgress after compaction: %v", err)
	}
	value, err = value.Terminate(Termination{
		Outcome: OutcomeCompleted, FinishedAt: createdAt.Add(3 * time.Second), MessageMark: 1,
	})
	if err != nil {
		t.Fatalf("Terminate: %v", err)
	}
	if got := value.ContextTokens(); got != 87_900 {
		t.Fatalf("terminal ContextTokens = %d, want 87900", got)
	}
}

func TestRunTerminalFactsRemainCoherent(t *testing.T) {
	tests := []struct {
		name    string
		outcome Outcome
		failure *Failure
		detail  string
		wantErr bool
	}{
		{name: "completed", outcome: OutcomeCompleted},
		{name: "completed with detail", outcome: OutcomeCompleted, detail: "unexpected", wantErr: true},
		{name: "failed", outcome: OutcomeFailed, failure: &Failure{Kind: FailureProviderRejected}},
		{name: "generic failure classified as lost", outcome: OutcomeFailed, failure: &Failure{Kind: FailureLost}, wantErr: true},
		{name: "failed with duplicate detail", outcome: OutcomeFailed, failure: &Failure{Kind: FailureProviderRejected}, detail: "duplicate", wantErr: true},
		{name: "failure missing", outcome: OutcomeFailed, wantErr: true},
		{name: "timeout classified", outcome: OutcomeTimedOut, failure: &Failure{Kind: FailureTimeout}},
		{name: "timeout misclassified", outcome: OutcomeTimedOut, failure: &Failure{Kind: FailureInternal}, wantErr: true},
		{name: "lost classified", outcome: OutcomeLost, failure: &Failure{Kind: FailureLost}},
		{name: "policy stop carries detail", outcome: OutcomeMaxSteps, detail: "limit reached"},
		{name: "cancellation carries detail", outcome: OutcomeCanceled, detail: "user stopped"},
		{name: "success carries failure", outcome: OutcomeCompleted, failure: &Failure{Kind: FailureInternal}, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value, err := Admit(Draft{
				RunID: "run_1", SessionID: "session_1", SegmentID: "segment_1",
				CreatedAt: time.Unix(1, 0).UTC(),
			})
			if err != nil {
				t.Fatalf("Admit: %v", err)
			}
			terminal, err := value.Terminate(Termination{
				Outcome: test.outcome, Failure: test.failure, Detail: test.detail,
				FinishedAt: time.Unix(2, 0).UTC(), MessageMark: 0,
			})
			if test.wantErr {
				if err == nil {
					t.Fatalf("Terminate accepted incoherent facts: %+v", terminal.Snapshot())
				}
				return
			}
			if err != nil {
				t.Fatalf("Terminate: %v", err)
			}
			if outcome, ok := terminal.Outcome(); !ok || outcome != test.outcome ||
				!terminal.FinishedAt().Equal(time.Unix(2, 0).UTC()) || terminal.MessageMark() != 0 {
				t.Fatalf("terminal Run = %+v", terminal.Snapshot())
			}
		})
	}
}

func TestForkReidentifiesTerminalHistoryAndClearsGoalAttribution(t *testing.T) {
	createdAt := time.Unix(1, 0).UTC()
	source, err := Admit(Draft{
		RunID: "run_source", SessionID: "session_source", SegmentID: "segment_source",
		GoalIncarnationID: "goal_incarnation_source", CreatedAt: createdAt,
	})
	if err != nil {
		t.Fatalf("Admit: %v", err)
	}
	source, err = source.Terminate(Termination{
		Outcome: OutcomeCompleted, FinishedAt: createdAt.Add(time.Second), MessageMark: 2,
	})
	if err != nil {
		t.Fatalf("Terminate: %v", err)
	}

	forked, err := source.Fork("session_child", "run_child_copy", Lineage{})
	if err != nil {
		t.Fatalf("Fork: %v", err)
	}
	if forked.SessionID() != "session_child" || forked.ID() != "run_child_copy" ||
		forked.GoalIncarnationID() != "" || forked.State() != Completed ||
		forked.MessageMark() != source.MessageMark() || !forked.CreatedAt().Equal(source.CreatedAt()) {
		t.Fatalf("forked Run = %+v", forked.Snapshot())
	}
	if source.SessionID() != "session_source" || source.ID() != "run_source" ||
		source.GoalIncarnationID() != "goal_incarnation_source" {
		t.Fatalf("fork mutated source Run: %+v", source.Snapshot())
	}
	if running, err := Admit(Draft{
		RunID: "run_running", SessionID: "session_source", SegmentID: "segment_running",
		CreatedAt: createdAt,
	}); err != nil {
		t.Fatalf("Admit running: %v", err)
	} else if _, err := running.Fork("session_child", "run_invalid", Lineage{}); err == nil {
		t.Fatal("Fork accepted a non-terminal Run")
	}
}

func TestRunRejectsIllegalTransitionsAndRegressingFacts(t *testing.T) {
	createdAt := time.Unix(2, 0).UTC()
	value, err := Admit(Draft{
		RunID: "run_1", SessionID: "session_1", SegmentID: "segment_1", CreatedAt: createdAt,
	})
	if err != nil {
		t.Fatalf("Admit: %v", err)
	}
	if _, resumeErr := value.Resume("segment_2", createdAt); resumeErr == nil {
		t.Fatal("running Run resumed")
	}
	if _, suspendErr := value.Suspend(createdAt.Add(-time.Nanosecond)); suspendErr == nil {
		t.Fatal("Run accepted a transition before its last update")
	}
	advanced, err := NewMetrics(nil, 2, time.Second)
	if err != nil {
		t.Fatalf("NewMetrics: %v", err)
	}
	value, err = value.AdvanceProgress(advanced, 0, createdAt.Add(time.Second))
	if err != nil {
		t.Fatalf("AdvanceMetrics: %v", err)
	}
	regressed, err := NewMetrics(nil, 1, time.Second)
	if err != nil {
		t.Fatalf("NewMetrics: %v", err)
	}
	if _, advanceProgressErr := value.AdvanceProgress(regressed, 0, createdAt.Add(2*time.Second)); advanceProgressErr == nil {
		t.Fatal("Run accepted regressing metrics")
	}
	terminal, err := value.Terminate(Termination{
		Outcome: OutcomeCompleted, FinishedAt: createdAt.Add(2 * time.Second), MessageMark: UnknownMessageMark,
	})
	if err != nil {
		t.Fatalf("Terminate: %v", err)
	}
	if _, err := terminal.Suspend(terminal.UpdatedAt()); err == nil {
		t.Fatal("terminal Run suspended")
	}
	if _, err := terminal.AdvanceProgress(advanced, 0, terminal.UpdatedAt()); err == nil {
		t.Fatal("terminal Run advanced metrics")
	}
}

func TestRunAndMetricsCopyOwnership(t *testing.T) {
	capabilities := Capabilities{InterruptKinds: []interrupt.Kind{interrupt.Question}}
	value, err := Admit(Draft{
		RunID: "run_1", SessionID: "session_1", SegmentID: "segment_1",
		Capabilities: capabilities, CreatedAt: time.Unix(1, 0).UTC(),
	})
	if err != nil {
		t.Fatalf("Admit: %v", err)
	}
	capabilities.InterruptKinds[0] = interrupt.Approval
	read := value.Capabilities()
	read.InterruptKinds[0] = interrupt.Approval
	if got := value.Capabilities().InterruptKinds[0]; got != interrupt.Question {
		t.Fatalf("Run capabilities changed through an external slice: %s", got)
	}

	cost := 1.0
	usage := accounting.Usage{
		Total:   accounting.Totals{CostUSD: &cost},
		ByModel: map[string]accounting.Totals{"model": {InputTokens: 1}},
	}
	metrics, err := NewMetrics(&usage, 1, 0)
	if err != nil {
		t.Fatalf("NewMetrics: %v", err)
	}
	usage.ByModel["model"] = accounting.Totals{InputTokens: 99}
	reported, ok := metrics.Usage()
	if !ok {
		t.Fatal("metrics lost usage")
	}
	reported.ByModel["model"] = accounting.Totals{InputTokens: 100}
	reportedAgain, _ := metrics.Usage()
	if got := reportedAgain.ByModel["model"].InputTokens; got != 1 {
		t.Fatalf("metrics usage changed through an external map: %d", got)
	}
}

func TestMetricsRejectsDurationOverflow(t *testing.T) {
	metrics, err := NewMetrics(nil, 0, time.Duration(math.MaxInt64))
	if err != nil {
		t.Fatalf("NewMetrics: %v", err)
	}
	if _, err := metrics.AddActiveDuration(time.Nanosecond); err == nil {
		t.Fatal("AddActiveDuration accepted overflow")
	}
}
