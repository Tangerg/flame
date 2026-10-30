package run

import (
	"testing"
	"time"
)

func TestReplacementChainsNamedTransitionsFromItsExpectedRun(t *testing.T) {
	createdAt := time.Date(2026, 9, 4, 1, 0, 0, 0, time.UTC)
	expected, err := Admit(Draft{
		RunID: "run_expected", SessionID: "session", SegmentID: "segment",
		ModelSelection: mustRunSelection(t), CreatedAt: createdAt,
	})
	if err != nil {
		t.Fatalf("Admit: %v", err)
	}
	if err := (Replacement{}).Validate(); err == nil {
		t.Fatal("a Replacement that was never started validated")
	}
	metrics, err := NewMetrics(nil, 2, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	progressed, err := expected.Replace().AdvanceProgress(metrics, 10, createdAt.Add(time.Second))
	if err != nil {
		t.Fatalf("AdvanceProgress: %v", err)
	}
	lost, err := progressed.RecoverLost(Failure{Kind: FailureLost}, createdAt.Add(time.Minute), MessageMarkAt(0))
	if err != nil {
		t.Fatalf("RecoverLost: %v", err)
	}
	if !lost.Expected().Equal(expected) || lost.State().Metrics().Steps() != 2 || !lost.State().State().IsTerminal() {
		t.Fatalf("replacement = %+v, want the admitted Run and its progressed lost successor", lost)
	}
	if _, err := lost.Resume("segment_next", createdAt.Add(2*time.Minute)); err == nil {
		t.Fatal("a terminal Replacement resumed")
	}
}
