package run

import (
	"testing"
	"time"
)

func TestReplacementCarriesTheProgressItsTerminalDecisionArrivedWith(t *testing.T) {
	createdAt := time.Date(2026, 9, 4, 1, 0, 0, 0, time.UTC)
	finishedAt := createdAt.Add(time.Minute)
	expected, err := Admit(Draft{
		RunID: "run_progress", SessionID: "session", SegmentID: "segment",
		ModelSelection: mustRunSelection(t), CreatedAt: createdAt,
	})
	if err != nil {
		t.Fatalf("Admit: %v", err)
	}
	metrics, err := NewMetrics(nil, 4, time.Second)
	if err != nil {
		t.Fatalf("NewMetrics: %v", err)
	}
	advanced, err := expected.AdvanceProgress(metrics, 1_200, finishedAt)
	if err != nil {
		t.Fatalf("AdvanceProgress: %v", err)
	}
	lost, err := advanced.RecoverLost(Failure{Kind: FailureLost}, finishedAt, 0)
	if err != nil {
		t.Fatalf("RecoverLost: %v", err)
	}
	replacement, err := NewReplacement(expected, lost)
	if err != nil {
		t.Fatalf("NewReplacement: %v", err)
	}
	// A Run is still running when it is replaced, so the last metrics its
	// executor committed arrive with the terminal decision rather than before it.
	if err := replacement.ValidateDerivedBy(func(current Run) (Run, error) {
		return current.RecoverLost(Failure{Kind: FailureLost}, finishedAt, 0)
	}); err != nil {
		t.Fatalf("ValidateDerivedBy refused the progress the decision arrived with: %v", err)
	}
	// The same replacement under a different terminal decision is a different Run.
	if err := replacement.ValidateDerivedBy(func(current Run) (Run, error) {
		return current.Terminate(Termination{Outcome: OutcomeCompleted, FinishedAt: finishedAt})
	}); err == nil {
		t.Fatal("ValidateDerivedBy accepted a state its transition does not produce")
	}
}

func TestReplacementRequiresOneValidRunIdentity(t *testing.T) {
	createdAt := time.Date(2026, 9, 4, 1, 0, 0, 0, time.UTC)
	expected, err := Admit(Draft{
		RunID: "run_expected", SessionID: "session", SegmentID: "segment",
		ModelSelection: mustRunSelection(t), CreatedAt: createdAt,
	})
	if err != nil {
		t.Fatalf("Admit: %v", err)
	}
	state, err := expected.RecoverLost(
		Failure{Kind: FailureLost},
		createdAt.Add(time.Minute),
		0,
	)
	if err != nil {
		t.Fatalf("RecoverLost: %v", err)
	}

	replacement, err := NewReplacement(expected, state)
	if err != nil {
		t.Fatalf("NewReplacement: %v", err)
	}
	if !replacement.Expected().Equal(expected) || !replacement.State().Equal(state) {
		t.Fatalf("replacement = %+v, want exact expected and state", replacement)
	}

	foreign := state.Snapshot()
	foreign.ID = "run_foreign"
	foreignState, err := Restore(foreign)
	if err != nil {
		t.Fatalf("Restore foreign state: %v", err)
	}
	if _, err := NewReplacement(expected, foreignState); err == nil {
		t.Fatal("NewReplacement accepted a different Run identity")
	}
	if _, err := NewReplacement(Run{}, state); err == nil {
		t.Fatal("NewReplacement accepted an invalid expected state")
	}
}
