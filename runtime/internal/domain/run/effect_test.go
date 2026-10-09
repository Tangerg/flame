package run

import (
	json "encoding/json/v2"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/scope/core/chat"
)

func TestUnresolvedEffectConstructionAndTerminalOwnership(t *testing.T) {
	for _, values := range [][5]string{
		{"", "effect", "cause", "", ""},
		{"process", "", "cause", "", ""},
		{"process", "effect", "", "", ""},
		{"process", "effect", "cause", "", strings.Repeat("x", 4097)},
	} {
		if _, err := NewUnresolvedEffect(UnresolvedEffectConfig{ProcessID: values[0], EffectID: values[1], Cause: values[2], Reason: values[3], Detail: values[4]}); err == nil {
			t.Fatal("invalid evidence accepted")
		}
	}
	effect, err := NewUnresolvedEffect(UnresolvedEffectConfig{ProcessID: "process", EffectID: "effect", Cause: "host_cancellation", Reason: "stop", Detail: "unknown"})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := Snapshot{SessionID: "session_1", ID: "run_1", ModelSelection: mustRunSelection(t), State: Running, ActiveSegmentID: "segment_1", CreatedAt: time.Unix(1, 0), UpdatedAt: time.Unix(1, 0), MessageMark: UnknownMessageMark()}
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

func TestUnresolvedEffectOwnsObservedOutput(t *testing.T) {
	for _, observed := range []chat.ToolOutput{
		{},
		{Content: []chat.ToolContent{{Kind: chat.PartText, Text: "observed"}}, Details: []byte(`{"receipt":9007199254740993}`)},
	} {
		original := observed.Clone()
		encoded, err := json.Marshal(original)
		if err != nil {
			t.Fatal(err)
		}
		effect, err := NewUnresolvedEffect(UnresolvedEffectConfig{ProcessID: "process", EffectID: "effect", Cause: "host_cancellation", Output: &observed})
		if err != nil {
			t.Fatal(err)
		}
		if len(observed.Content) > 0 {
			observed.Content[0].Text = "changed input"
			observed.Details[0] = 'x'
		}
		projection := effect.Output()
		if string(projection) != string(encoded) {
			t.Fatalf("borrowed output: %s, want %s", projection, encoded)
		}
		without, err := NewUnresolvedEffect(UnresolvedEffectConfig{ProcessID: "process", EffectID: "effect", Cause: "host_cancellation"})
		if err != nil || without.Output() != "" || without == effect {
			t.Fatalf("absent output lost its distinction from known output: %v", err)
		}
	}
	invalid := chat.ToolOutput{Content: []chat.ToolContent{{Kind: chat.PartToolCall}}}
	if _, err := NewUnresolvedEffect(UnresolvedEffectConfig{ProcessID: "process", EffectID: "effect", Cause: "host_cancellation", Output: &invalid}); err == nil {
		t.Fatal("invalid Scope Tool output was accepted")
	}
}
