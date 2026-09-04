package run

import (
	"testing"
	"time"
)

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
