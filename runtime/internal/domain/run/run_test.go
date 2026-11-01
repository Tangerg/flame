package run

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
)

func mustRunSelection(t testing.TB) modelchoice.Selection {
	t.Helper()
	selection, err := modelchoice.New("provider", "model")
	if err != nil {
		t.Fatalf("modelchoice.New: %v", err)
	}
	return selection
}

func TestRunTransitionsRefuseUnrepresentableInstants(t *testing.T) {
	current, err := Admit(Draft{
		RunID: "run_time", SessionID: "ses_time", SegmentID: "seg_time",
		ModelSelection: mustRunSelection(t), CreatedAt: time.Unix(1, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	waiting, err := current.Suspend(time.Unix(1, 0))
	if err != nil {
		t.Fatal(err)
	}
	at := time.Unix(0, math.MaxInt64).Add(time.Nanosecond)
	for _, test := range []struct {
		name    string
		advance func() (Run, error)
	}{
		{"progress", func() (Run, error) { return current.AdvanceProgress(current.Metrics(), 1, at) }},
		{"suspend", func() (Run, error) { return current.Suspend(at) }},
		{"resume", func() (Run, error) { return waiting.Resume("seg_next", at) }},
		{"terminal", func() (Run, error) {
			return current.Terminate(Termination{Ending: Completed{}, FinishedAt: at, MessageMark: MessageMarkAt(0)})
		}},
		{"lost", func() (Run, error) { return waiting.RecoverLost(Loss{Reason: LossRestart}, at, MessageMarkAt(0)) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := test.advance(); err == nil || !strings.Contains(err.Error(), "outside the exact Unix nanosecond range") {
				t.Fatalf("transition error = %v, want unrepresentable instant refusal", err)
			}
		})
	}
}

func TestRestoreRejectsInvalidState(t *testing.T) {
	snapshot := Snapshot{
		SessionID: "session_1", ID: "run_1", ModelSelection: mustRunSelection(t),
		State: Waiting, CreatedAt: time.Unix(1, 0), UpdatedAt: time.Unix(1, 0),
		MessageMark: UnknownMessageMark(),
	}
	if _, err := Restore(snapshot); err != nil {
		t.Fatalf("valid waiting Run: %v", err)
	}
	for _, state := range []State{"", "unknown"} {
		t.Run(string(state), func(t *testing.T) {
			snapshot.State = state
			if _, err := Restore(snapshot); err == nil {
				t.Fatalf("Restore accepted state %q", state)
			}
		})
	}
}

func TestChildRunCarriesNoCapabilitiesOfItsOwn(t *testing.T) {
	lineage := Lineage{spawnedByItemID: "item_1", parentRunID: "run_root", rootRunID: "run_root"}
	capabilities := Capabilities{InterruptTypes: []InterruptType{InterruptQuestion}}
	draft := Draft{
		RunID: "run_child", SessionID: "session_1", SegmentID: "segment_1",
		SpawnedByItemID: lineage.SpawnedByItemID(), ParentRunID: lineage.ParentRunID(), RootRunID: lineage.RootRunID(),
		ModelSelection: mustRunSelection(t), CreatedAt: time.Unix(1, 0), Capabilities: capabilities,
	}
	if _, err := Admit(draft); err == nil {
		t.Fatal("Admit accepted a child stating its root's capabilities")
	}
	if _, err := Restore(Snapshot{
		SessionID: "session_1", ID: "run_child", Lineage: lineage, ModelSelection: mustRunSelection(t),
		State: Waiting, Capabilities: capabilities, CreatedAt: time.Unix(1, 0), UpdatedAt: time.Unix(1, 0),
		MessageMark: UnknownMessageMark(),
	}); err == nil {
		t.Fatal("Restore accepted a child stating its root's capabilities")
	}
}

func TestRunAdmissionRejectsNonCanonicalOrUnboundedResourceIdentity(t *testing.T) {
	valid := Draft{
		RunID: "run_1", SessionID: "session_1", SegmentID: "segment_1",
		ModelSelection: mustRunSelection(t), CreatedAt: time.Unix(1, 0),
	}
	for name, mutate := range map[string]func(*Draft){
		"run control":       func(draft *Draft) { draft.RunID = "run_\n1" },
		"session control":   func(draft *Draft) { draft.SessionID = "session_\t1" },
		"segment oversized": func(draft *Draft) { draft.SegmentID = strings.Repeat("s", identity.MaximumResourceCharacters+1) },
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

func TestRunAdmissionRequiresExactModelSelection(t *testing.T) {
	t.Parallel()

	_, err := Admit(Draft{
		RunID: "run_1", SessionID: "session_1", SegmentID: "segment_1",
		CreatedAt: time.Unix(1, 0).UTC(),
	})
	if err == nil || !strings.Contains(err.Error(), "model selection is required") {
		t.Fatalf("Admit without model selection error = %v", err)
	}
}

func TestRunLifecyclePreservesAdmissionFactsAndAdvancesMetrics(t *testing.T) {
	createdAt := time.Unix(1, 0).UTC()
	capabilities := Capabilities{ChildRuns: true, InterruptTypes: []InterruptType{InterruptQuestion}}
	value, err := Admit(Draft{
		RunID: "run_1", SessionID: "session_1", SegmentID: "segment_1",
		ModelSelection: mustRunSelection(t), Capabilities: capabilities, CreatedAt: createdAt,
	})
	if err != nil {
		t.Fatalf("Admit: %v", err)
	}
	cost, err := accounting.NewCost(0.25)
	if err != nil {
		t.Fatal(err)
	}
	metrics, err := NewMetrics(&accounting.Usage{
		Total: accounting.Totals{InputTokens: 3, OutputTokens: 2, Cost: cost},
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
		RunID: "run_1", SessionID: "session_1", SegmentID: "segment_1",
		ModelSelection: mustRunSelection(t), CreatedAt: createdAt,
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
		Ending: Completed{}, FinishedAt: createdAt.Add(3 * time.Second), MessageMark: MessageMarkAt(1),
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
		ending  Ending
		wantErr bool
	}{
		{name: "completed", ending: Completed{}},
		{name: "failed", ending: Failed{Failure: Failure{Kind: FailureProviderRejected}}},
		{name: "failure unclassified", ending: Failed{}, wantErr: true},
		{name: "timed out", ending: TimedOut{Detail: "deadline reached"}},
		{name: "lost", ending: Lost{Loss: Loss{Reason: LossUnresolvedEffects}}},
		{name: "loss unexplained", ending: Lost{}, wantErr: true},
		{name: "cancellation carries detail", ending: Canceled{Detail: "stopped by user"}},
		{name: "cancellation detail not canonical", ending: Canceled{Detail: " stopped "}, wantErr: true},
		{name: "no ending", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value, err := Admit(Draft{
				RunID: "run_1", SessionID: "session_1", SegmentID: "segment_1",
				ModelSelection: mustRunSelection(t), CreatedAt: time.Unix(1, 0).UTC(),
			})
			if err != nil {
				t.Fatalf("Admit: %v", err)
			}
			terminal, err := value.Terminate(Termination{
				Ending: test.ending, FinishedAt: time.Unix(2, 0).UTC(), MessageMark: MessageMarkAt(0),
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
			if outcome, ok := terminal.Outcome(); !ok || outcome != test.ending.Outcome() ||
				!terminal.FinishedAt().Equal(time.Unix(2, 0).UTC()) || terminal.MessageMark() != MessageMarkAt(0) {
				t.Fatalf("terminal Run = %+v", terminal.Snapshot())
			}
		})
	}
}

func TestForkReidentifiesTerminalHistoryAndClearsGoalAttribution(t *testing.T) {
	createdAt := time.Unix(1, 0).UTC()
	source, err := Admit(Draft{
		RunID: "run_source", SessionID: "session_source", SegmentID: "segment_source",
		ModelSelection: mustRunSelection(t), GoalIncarnationID: "goal_incarnation_source", CreatedAt: createdAt,
	})
	if err != nil {
		t.Fatalf("Admit: %v", err)
	}
	source, err = source.Terminate(Termination{
		Ending: Completed{}, FinishedAt: createdAt.Add(time.Second), MessageMark: MessageMarkAt(2),
	})
	if err != nil {
		t.Fatalf("Terminate: %v", err)
	}

	childSession, err := identity.ParseSession("session_child")
	if err != nil {
		t.Fatal(err)
	}
	childRun, err := identity.ParseRun("run_child_copy")
	if err != nil {
		t.Fatal(err)
	}
	forked, err := source.Fork(childSession, childRun, Lineage{})
	if err != nil {
		t.Fatalf("Fork: %v", err)
	}
	if forked.SessionID() != "session_child" || forked.ID() != "run_child_copy" ||
		forked.GoalIncarnationID() != "" || forked.State() != Finished ||
		forked.MessageMark() != source.MessageMark() || !forked.CreatedAt().Equal(source.CreatedAt()) {
		t.Fatalf("forked Run = %+v", forked.Snapshot())
	}
	if source.SessionID() != "session_source" || source.ID() != "run_source" ||
		source.GoalIncarnationID() != "goal_incarnation_source" {
		t.Fatalf("fork mutated source Run: %+v", source.Snapshot())
	}
	if running, err := Admit(Draft{
		RunID: "run_running", SessionID: "session_source", SegmentID: "segment_running",
		ModelSelection: mustRunSelection(t), CreatedAt: createdAt,
	}); err != nil {
		t.Fatalf("Admit running: %v", err)
	} else if _, err := running.Fork(childSession, childRun, Lineage{}); err == nil {
		t.Fatal("Fork accepted a non-terminal Run")
	}
}

func TestForkKeepsTheLineageKindItWasDerivedFrom(t *testing.T) {
	createdAt := time.Unix(1, 0).UTC()
	root, err := Admit(Draft{
		RunID: "run_root", SessionID: "session_source", SegmentID: "segment_root",
		ModelSelection: mustRunSelection(t), CreatedAt: createdAt,
	})
	if err != nil {
		t.Fatalf("Admit root: %v", err)
	}
	root, err = root.Terminate(Termination{
		Ending: Completed{}, FinishedAt: createdAt.Add(time.Second), MessageMark: MessageMarkAt(1),
	})
	if err != nil {
		t.Fatalf("Terminate root: %v", err)
	}
	// Lineage.Validate proves a lineage is well formed, not that it names the
	// same kind of Run. Only Fork can refuse to turn a root into a child.
	childLineage := Lineage{spawnedByItemID: "item_spawn", parentRunID: "run_parent", rootRunID: "run_parent"}
	if err := childLineage.Validate("run_child_copy"); err != nil {
		t.Fatalf("child lineage is not well formed: %v", err)
	}
	childSession, err := identity.ParseSession("session_child")
	if err != nil {
		t.Fatal(err)
	}
	childRun, err := identity.ParseRun("run_child_copy")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := root.Fork(childSession, childRun, childLineage); err == nil {
		t.Fatal("Fork turned a root Run into a child")
	}
}

func TestProgressRefusesANegativePromptFootprintInsteadOfIgnoringIt(t *testing.T) {
	createdAt := time.Unix(3, 0).UTC()
	value, err := Admit(Draft{
		RunID: "run_footprint", SessionID: "session_footprint", SegmentID: "segment_footprint",
		ModelSelection: mustRunSelection(t), CreatedAt: createdAt,
	})
	if err != nil {
		t.Fatalf("Admit: %v", err)
	}
	// AdvanceProgress stores a footprint only when it is positive, so a negative
	// one would otherwise be dropped and reported as a committed boundary.
	if _, err := value.AdvanceProgress(value.Metrics(), -1, createdAt.Add(time.Second)); err == nil {
		t.Fatal("AdvanceProgress accepted a negative prompt footprint")
	}
}

// TestAdvanceProgressRefusesAValueOutsideTheStateMachine: Go's zero value is a
// construction path the aggregate cannot forbid, so the transition itself has to
// refuse a state no Run ever reached.
func TestAdvanceProgressRefusesAValueOutsideTheStateMachine(t *testing.T) {
	if _, err := (Run{}).AdvanceProgress(Metrics{}, 0, time.Unix(1, 0).UTC()); err == nil {
		t.Fatal("AdvanceProgress advanced a zero-value Run")
	}
}

func TestCancelWaitingRefusesARunThatIsNotWaiting(t *testing.T) {
	createdAt := time.Unix(4, 0).UTC()
	value, err := Admit(Draft{
		RunID: "run_waiting", SessionID: "session_waiting", SegmentID: "segment_waiting",
		ModelSelection: mustRunSelection(t), CreatedAt: createdAt,
	})
	if err != nil {
		t.Fatalf("Admit: %v", err)
	}
	if _, err := value.CancelWaiting("", createdAt.Add(time.Second), MessageMarkAt(0)); err == nil {
		t.Fatal("CancelWaiting canceled a Running Run")
	}
	suspended, err := value.Suspend(createdAt.Add(time.Second))
	if err != nil {
		t.Fatalf("Suspend: %v", err)
	}
	if _, err := suspended.CancelWaiting("", createdAt.Add(2*time.Second), MessageMarkAt(0)); err != nil {
		t.Fatalf("CancelWaiting refused a Waiting Run: %v", err)
	}
}

func TestRunRejectsIllegalTransitionsAndRegressingFacts(t *testing.T) {
	createdAt := time.Unix(2, 0).UTC()
	value, err := Admit(Draft{
		RunID: "run_1", SessionID: "session_1", SegmentID: "segment_1",
		ModelSelection: mustRunSelection(t), CreatedAt: createdAt,
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
		Ending: Completed{}, FinishedAt: createdAt.Add(2 * time.Second), MessageMark: UnknownMessageMark(),
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
	capabilities := Capabilities{InterruptTypes: []InterruptType{InterruptQuestion}}
	value, err := Admit(Draft{
		RunID: "run_1", SessionID: "session_1", SegmentID: "segment_1",
		ModelSelection: mustRunSelection(t), Capabilities: capabilities, CreatedAt: time.Unix(1, 0).UTC(),
	})
	if err != nil {
		t.Fatalf("Admit: %v", err)
	}
	capabilities.InterruptTypes[0] = InterruptApproval
	read := value.Capabilities()
	read.InterruptTypes[0] = InterruptApproval
	if got := value.Capabilities().InterruptTypes[0]; got != InterruptQuestion {
		t.Fatalf("Run capabilities changed through an external slice: %s", got)
	}

	cost, err := accounting.NewCost(1.0)
	if err != nil {
		t.Fatal(err)
	}
	usage := accounting.Usage{
		Total:   accounting.Totals{Cost: cost},
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

// TestMetricsRejectsDurationOverflow covers the accumulation the reducer
// actually performs: it adds a segment's duration to the running total with
// ordinary arithmetic, so an overflow arrives here as a wrapped negative value
// and NewMetrics is what must refuse it.
func TestMetricsRejectsDurationOverflow(t *testing.T) {
	total := time.Duration(math.MaxInt64)
	wrapped := total + time.Duration(1)
	if wrapped >= 0 {
		t.Fatalf("fixture did not wrap: %d", wrapped)
	}
	if _, err := NewMetrics(nil, 0, wrapped); err == nil {
		t.Fatal("NewMetrics accepted an overflowed active duration")
	}
}

// TestRestoreRejectsSnapshotsThatBreakRunInvariants covers the decode path
// rather than the transitions. Terminate derives the terminal state from the
// outcome, so it cannot produce a mismatched pair; a durable row can. These are
// the invariants outer layers rely on instead of re-checking a restored Run.
func TestRestoreRejectsSnapshotsThatBreakRunInvariants(t *testing.T) {
	createdAt := time.Unix(1, 0).UTC()
	finishedAt := time.Unix(2, 0).UTC()
	valid := func() Snapshot {
		return Snapshot{
			SessionID: "session_1", ID: "run_1", ModelSelection: mustRunSelection(t),
			State: Finished, Ending: Completed{},
			CreatedAt: createdAt, FinishedAt: finishedAt, UpdatedAt: finishedAt,
		}
	}
	for _, test := range []struct {
		name   string
		break_ func(*Snapshot)
	}{
		{name: "terminal without an ending", break_: func(s *Snapshot) { s.Ending = nil }},
		{name: "terminal without a finish time", break_: func(s *Snapshot) { s.FinishedAt = time.Time{} }},
		{name: "terminal carries an active segment", break_: func(s *Snapshot) { s.ActiveSegmentID = "segment_1" }},
		{name: "running without an active segment", break_: func(s *Snapshot) {
			*s = Snapshot{
				SessionID: s.SessionID, ID: s.ID, ModelSelection: s.ModelSelection,
				State: Running, CreatedAt: createdAt, UpdatedAt: createdAt,
				MessageMark: UnknownMessageMark(),
			}
		}},
		{name: "lost without a reason", break_: func(s *Snapshot) { s.Ending = Lost{} }},
		{name: "open Run carries an ending", break_: func(s *Snapshot) {
			s.State, s.ActiveSegmentID, s.FinishedAt = Running, "segment_1", time.Time{}
		}},
		{name: "negative prompt footprint", break_: func(s *Snapshot) { s.ContextTokens = -1 }},
		{name: "malformed active segment", break_: func(s *Snapshot) {
			*s = Snapshot{
				SessionID: s.SessionID, ID: s.ID, ModelSelection: s.ModelSelection,
				State: Running, ActiveSegmentID: " segment_1", CreatedAt: createdAt,
				UpdatedAt: createdAt, MessageMark: UnknownMessageMark(),
			}
		}},
		{name: "failure the Failure value rejects", break_: func(s *Snapshot) {
			s.Ending = Failed{Failure: Failure{Kind: FailureProviderRejected, RetryAfter: -1}}
		}},
		{name: "child carries a Goal incarnation", break_: func(s *Snapshot) {
			s.Lineage = Lineage{
				spawnedByItemID: "item_parent", parentRunID: "run_parent", rootRunID: "run_root",
			}
			s.GoalIncarnationID = "incarnation_1"
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			snapshot := valid()
			test.break_(&snapshot)
			if restored, err := Restore(snapshot); err == nil {
				t.Fatalf("Restore accepted an incoherent snapshot: %+v", restored.Snapshot())
			}
		})
	}
	if _, err := Restore(valid()); err != nil {
		t.Fatalf("Restore rejected a coherent snapshot: %v", err)
	}
}
