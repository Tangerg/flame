package conversation

import (
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/builtin"
)

func TestRunEventValidationOwnsEnvelopeAndPayloadIdentity(t *testing.T) {
	t.Parallel()

	valid := RunEvent{
		EventID: "event_1", RunID: "run_1", SegmentID: "segment_1", At: time.Now(),
		Event: BlockCompleted{Block: Block{
			ID: "answer", RunID: "run_1", Status: BlockStatusCompleted, Kind: BlockAssistant, Text: "done",
		}},
	}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		mutate func(*RunEvent)
	}{
		{name: "stream segment id", mutate: func(event *RunEvent) { event.StreamSegmentID = " " }},
		{name: "payload", mutate: func(event *RunEvent) { event.Event = nil }},
		{name: "block lifecycle", mutate: func(event *RunEvent) {
			completed := event.Event.(BlockCompleted)
			completed.Block.Status = BlockStatusRunning
			event.Event = completed
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			candidate := valid.Clone()
			test.mutate(&candidate)
			if err := candidate.Validate(); err == nil {
				t.Fatalf("RunEvent.Validate accepted %+v", candidate)
			}
		})
	}

	started := RunEvent{
		EventID: "event_2", RunID: "run_1", SegmentID: "segment_1", At: time.Now(),
		Event: SegmentStarted{Run: runningRun("segment_1")},
	}
	if err := started.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestEphemeralEventValidationRejectsMalformedValues(t *testing.T) {
	for _, event := range []Event{
		BlockDelta{},
		CustomEvent{Name: "vendor.trace", PayloadJSON: []byte(`{`)},
		CustomEvent{PayloadJSON: []byte(`null`)},
	} {
		if err := ValidateEvent(event); err == nil {
			t.Fatalf("ValidateEvent(%T) accepted %+v", event, event)
		}
	}
}

func TestRunningToolRejectsTerminalTiming(t *testing.T) {
	for _, call := range []ToolCall{
		{Kind: builtin.CategoryShell, Status: ToolRunning, Duration: time.Second},
		{Kind: builtin.CategoryShell, Status: ToolRunning, FinishedAt: time.Now()},
		{Kind: builtin.CategoryShell, Status: ToolOK, Duration: -time.Millisecond},
	} {
		if err := call.Validate(); err == nil {
			t.Fatalf("invalid tool timing was accepted: %+v", call)
		}
	}
}
