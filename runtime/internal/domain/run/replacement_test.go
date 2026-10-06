package run

import (
	"errors"
	"testing"
	"time"
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
