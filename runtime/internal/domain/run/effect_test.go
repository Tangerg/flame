package run

import (
	"strings"
	"testing"
	"time"
)

func TestUnresolvedEffectConstructionAndTerminalOwnership(t *testing.T) {
	for _, values := range [][5]string{
		{"", "effect", "cause", "", ""},
		{"process", "", "cause", "", ""},
		{"process", "effect", "", "", ""},
		{"process", "effect", "cause", "", strings.Repeat("x", 4097)},
	} {
		if _, err := NewUnresolvedEffect(values[0], values[1], values[2], values[3], values[4]); err == nil {
			t.Fatal("invalid evidence accepted")
		}
	}
	effect, err := NewUnresolvedEffect("process", "effect", "host_cancellation", "stop", "unknown")
	if err != nil {
		t.Fatal(err)
	}
	snapshot := Snapshot{SessionID: "session_1", ID: "run_1", ModelSelection: mustRunSelection(t), State: Running, ActiveSegmentID: "segment_1", CreatedAt: time.Unix(1, 0), UpdatedAt: time.Unix(1, 0), MessageMark: UnknownMessageMark}
	active, err := Restore(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	snapshot.UnresolvedEffects = []UnresolvedEffect{effect}
	if _, err := Restore(snapshot); err == nil {
		t.Fatal("open Run accepted terminal evidence")
	}
	for _, effects := range [][]UnresolvedEffect{{{}}, {effect, effect}} {
		if _, err := active.Terminate(Termination{Outcome: OutcomeCanceled, FinishedAt: time.Unix(2, 0), UnresolvedEffects: effects}); err == nil {
			t.Fatal("invalid terminal evidence accepted")
		}
	}
	evidence := []UnresolvedEffect{effect}
	terminal, err := active.Terminate(Termination{Outcome: OutcomeCanceled, FinishedAt: time.Unix(2, 0), UnresolvedEffects: evidence})
	if err != nil {
		t.Fatal(err)
	}
	evidence[0] = UnresolvedEffect{}
	if terminal.UnresolvedEffects()[0] != effect {
		t.Fatal("terminal borrowed caller storage")
	}
	restored, err := Restore(terminal.Snapshot())
	if err != nil || !restored.Equal(terminal) {
		t.Fatalf("snapshot lost evidence: %v", err)
	}
	without := terminal.Snapshot()
	without.UnresolvedEffects = nil
	different, err := Restore(without)
	if err != nil {
		t.Fatal(err)
	}
	if different.Equal(terminal) {
		t.Fatal("equality discarded execution evidence")
	}
}
