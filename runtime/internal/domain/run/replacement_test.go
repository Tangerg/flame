package run

import (
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
)

func TestReplaceDerivesTheStateFromItsExpectedRun(t *testing.T) {
	createdAt := time.Date(2026, 9, 4, 1, 0, 0, 0, time.UTC)
	finishedAt := createdAt.Add(time.Minute)
	expected, err := Admit(Draft{
		RunID: "run_expected", SessionID: "session", SegmentID: "segment",
		ModelSelection: mustRunSelection(t), CreatedAt: createdAt,
	})
	if err != nil {
		t.Fatalf("Admit: %v", err)
	}
	replacement, err := Replace(expected, func(current Run) (Run, error) {
		return current.RecoverLost(Failure{Kind: FailureLost}, finishedAt, MessageMarkAt(0))
	})
	if err != nil {
		t.Fatalf("Replace: %v", err)
	}
	if !replacement.Expected().Equal(expected) || !replacement.State().State().IsTerminal() {
		t.Fatalf("replacement = %+v, want the expected Run and its lost successor", replacement)
	}

	illegal := errors.New("illegal")
	if _, err := Replace(expected, func(Run) (Run, error) { return Run{}, illegal }); !errors.Is(err, illegal) {
		t.Fatalf("Replace transition error = %v, want %v", err, illegal)
	}
	if _, err := Replace(Run{}, func(current Run) (Run, error) { return current, nil }); err == nil {
		t.Fatal("Replace accepted an unbuilt expected Run")
	}
	foreign := replacement.State().Snapshot()
	foreign.ID = "run_foreign"
	foreignState, err := Restore(foreign)
	if err != nil {
		t.Fatalf("Restore foreign state: %v", err)
	}
	if _, err := Replace(expected, func(Run) (Run, error) { return foreignState, nil }); err == nil {
		t.Fatal("Replace accepted a transition that changed Run identity")
	}
	if _, err := replacement.Then(func(Run) (Run, error) { return foreignState, nil }); err == nil {
		t.Fatal("Then accepted a transition that changed Run identity")
	}
}

func TestReplacementCannotRewriteAdmission(t *testing.T) {
	createdAt := time.Date(2026, 10, 9, 1, 0, 0, 0, time.UTC)
	expected, err := Admit(Draft{
		RunID: "run_expected", SessionID: "session", SegmentID: "segment",
		ModelSelection: mustRunSelection(t), GoalIncarnationID: "goal_origin",
		CreatedAt: createdAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	selection, err := modelref.New("other", "model")
	if err != nil {
		t.Fatal(err)
	}
	for name, edit := range map[string]func(*Snapshot){
		"model selection": func(s *Snapshot) { s.ModelSelection = selection },
		"Goal origin":     func(s *Snapshot) { s.GoalIncarnationID = "replacement_goal" },
		"capabilities":    func(s *Snapshot) { s.Capabilities = Capabilities{ChildRuns: true} },
		"creation time":   func(s *Snapshot) { s.CreatedAt = createdAt.Add(-time.Minute) },
		"lineage": func(s *Snapshot) {
			s.Lineage = Lineage{ParentRunID: "run_parent", RootRunID: "run_parent", SpawnedByItemID: "item_spawn"}
			s.GoalIncarnationID = ""
		},
	} {
		t.Run(name, func(t *testing.T) {
			snapshot := expected.Snapshot()
			edit(&snapshot)
			reconstructed, err := Restore(snapshot)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := Replace(expected, func(Run) (Run, error) { return reconstructed, nil }); err == nil {
				t.Fatal("replacement rewrote an admission fact")
			}
		})
	}
}

func TestReplacementPreservesSettledFactsAcrossThen(t *testing.T) {
	createdAt := time.Date(2026, 10, 9, 1, 0, 0, 0, time.UTC)
	expected, err := Admit(Draft{
		RunID: "run_expected", SessionID: "session", SegmentID: "segment",
		ModelSelection: mustRunSelection(t), CreatedAt: createdAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	settled, err := Replace(expected, func(current Run) (Run, error) {
		return current.Terminate(Termination{Outcome: OutcomeCompleted, FinishedAt: createdAt.Add(time.Minute)})
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := settled.Then(func(Run) (Run, error) { return expected, nil }); err == nil {
		t.Fatal("Then reopened a completed Run")
	}
	changed := settled.State().Snapshot()
	changed.Outcome = new(OutcomeCanceled)
	changed.State = Canceled
	changedOutcome, err := Restore(changed)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := settled.Then(func(Run) (Run, error) { return changedOutcome, nil }); err == nil {
		t.Fatal("Then changed a settled outcome")
	}
	marked, err := settled.Then(func(current Run) (Run, error) { return current.WithMessageMark(7) })
	if err != nil {
		t.Fatal(err)
	}
	if mark, known := marked.State().MessageMark().Count(); !known || mark != 7 || !marked.Expected().Equal(expected) {
		t.Fatalf("watermark replacement = %+v", marked.State().Snapshot())
	}
	if _, err := marked.Then(func(Run) (Run, error) { return settled.State(), nil }); err == nil {
		t.Fatal("Then cleared a settled watermark")
	}
}

func TestReplacementUsesRunTransitionsForLiveFacts(t *testing.T) {
	createdAt := time.Date(2026, 10, 9, 1, 0, 0, 0, time.UTC)
	opened, err := Admit(Draft{
		RunID: "run_expected", SessionID: "session", SegmentID: "segment",
		ModelSelection: mustRunSelection(t), CreatedAt: createdAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	progressed, err := opened.AdvanceProgress(opened.Metrics(), 100, createdAt.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	for name, edit := range map[string]func(*Snapshot){
		"Segment substitution": func(snapshot *Snapshot) { snapshot.ActiveSegmentID = "segment_foreign" },
		"context erasure":      func(snapshot *Snapshot) { snapshot.ContextTokens = 0 },
		"time reversal":        func(snapshot *Snapshot) { snapshot.UpdatedAt = createdAt },
	} {
		t.Run(name, func(t *testing.T) {
			snapshot := progressed.Snapshot()
			edit(&snapshot)
			state, err := Restore(snapshot)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := Replace(progressed, func(Run) (Run, error) { return state, nil }); err == nil {
				t.Fatal("replacement bypassed the Run transition")
			}
		})
	}
	parked, err := Replace(progressed, func(current Run) (Run, error) { return current.Suspend(createdAt.Add(time.Second)) })
	if err != nil {
		t.Fatal(err)
	}
	resumed, err := parked.Then(func(current Run) (Run, error) { return current.Resume("segment_next", createdAt.Add(2*time.Second)) })
	if err != nil {
		t.Fatal(err)
	}
	if !resumed.Expected().Equal(progressed) || resumed.State().ActiveSegmentID() != "segment_next" {
		t.Fatalf("resumed = %+v", resumed.State().Snapshot())
	}
}
