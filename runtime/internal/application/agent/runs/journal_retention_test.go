package runs

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/scope/core/chat"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/domain/session/plan"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func TestRetentionChargeTracksEveryVariableReplayPayload(t *testing.T) {
	const growth = 32 << 10
	largeText := strings.Repeat("x", growth)
	largeResult, err := tool.NewResult(largeText)
	if err != nil {
		t.Fatal(err)
	}
	canceled := run.OutcomeCanceled
	observed := chat.ToolOutput{Content: []chat.ToolContent{{Kind: chat.PartText, Text: largeText}}}
	effect, err := run.NewUnresolvedEffect(run.UnresolvedEffectConfig{
		ProcessID: "process", EffectID: "effect", Cause: "host_cancellation", Output: &observed,
	})
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name  string
		small ProjectionEvent
		large ProjectionEvent
	}{
		{
			name:  "run",
			small: SegmentFinished{Run: testsupport.MustRestoreRun(run.Snapshot{ID: "run", State: run.Canceled, Outcome: &canceled})},
			large: SegmentFinished{Run: testsupport.MustRestoreRun(run.Snapshot{ID: "run", State: run.Canceled, Outcome: &canceled, Detail: largeText})},
		},
		{
			name:  "unresolved effect output",
			small: SegmentFinished{Run: testsupport.MustRestoreRun(run.Snapshot{ID: "run", State: run.Canceled, Outcome: &canceled})},
			large: SegmentFinished{Run: testsupport.MustRestoreRun(run.Snapshot{ID: "run", State: run.Canceled, Outcome: &canceled, UnresolvedEffects: []run.UnresolvedEffect{effect}})},
		},
		{
			name: "item start identity",
			small: ItemStarted{Item: ItemStart{
				SessionID: "session", RunID: "run", ItemID: "item",
				Kind: transcript.Reasoning, OccurredAt: time.Unix(1, 0),
			}},
			large: ItemStarted{Item: ItemStart{
				SessionID: "session", RunID: "run", ItemID: "item" + largeText,
				Kind: transcript.Reasoning, OccurredAt: time.Unix(1, 0),
			}},
		},
		{
			name:  "item media",
			small: ItemCompleted{Item: testsupport.MustRestoreItem(testsupport.ItemInput{ID: "item"})},
			large: ItemCompleted{Item: testsupport.MustRestoreItem(testsupport.ItemInput{ID: "item", Content: []transcript.ContentBlock{{Kind: transcript.ImageContent, MediaType: "image/png", Bytes: make([]byte, growth)}}})},
		},
		{
			name: "tool result",
			small: ItemCompleted{Item: testsupport.MustRestoreItem(testsupport.ItemInput{
				Kind: transcript.ToolCall, Status: transcript.ItemCompleted,
				Tool: &transcript.ToolInvocation{Name: "shell"},
			})},
			large: ItemCompleted{Item: testsupport.MustRestoreItem(testsupport.ItemInput{
				Kind: transcript.ToolCall, Status: transcript.ItemCompleted,
				Tool: &transcript.ToolInvocation{Name: "shell", Result: &largeResult},
			})},
		},
		{
			name:  "Plan snapshot",
			small: PlanSnapshot{SessionID: "session"},
			large: PlanSnapshot{SessionID: "session", Steps: []plan.Step{{Description: largeText, Status: plan.StatusPending}}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			small := test.small.retainedBytes()
			large := test.large.retainedBytes()
			if large-small < growth {
				t.Fatalf("large payload charge grew by %d bytes, want at least %d", large-small, growth)
			}
		})
	}
}

func TestJournalRetentionBoundsUnresolvedEffectEvidence(t *testing.T) {
	const budget = 8 << 10
	for _, test := range []struct {
		name   string
		config run.UnresolvedEffectConfig
	}{
		{
			name: "output",
			config: run.UnresolvedEffectConfig{
				ProcessID: "process", EffectID: "effect", Cause: "host_cancellation",
				Output: &chat.ToolOutput{Content: []chat.ToolContent{{Kind: chat.PartText, Text: strings.Repeat("x", budget*2)}}},
			},
		},
		{
			name: "diagnostics",
			config: run.UnresolvedEffectConfig{
				ProcessID: strings.Repeat("p", 512), EffectID: strings.Repeat("e", 512), Cause: strings.Repeat("c", 512),
				Reason: strings.Repeat("r", 4096), Detail: strings.Repeat("d", 4096),
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			effect, err := run.NewUnresolvedEffect(test.config)
			if err != nil {
				t.Fatal(err)
			}
			terminal := testsupport.MustRestoreRun(run.Snapshot{
				ID: testRunID, State: run.Canceled, UnresolvedEffects: []run.UnresolvedEffect{effect},
			})
			j := mustNewJournal(t, testStreamScope(testEpoch, testRunID, testSegmentID), Retention{MaxEvents: 1024, MaxBytes: budget})
			mustAppendJournal(t, j, ev(true))
			attached := j.tail()
			defer attached.Cancel()
			mustAppendJournal(t, j, Event{RunID: testRunID, SegmentID: testSegmentID, Payload: SegmentFinished{Run: terminal}})
			mustCloseJournal(t, j)
			if _, err := j.replay(cursorAt(t, 1)); !errors.Is(err, ErrReplayUnavailable) {
				t.Fatalf("oversized Run evidence stayed replayable: %v", err)
			}
			if got := drain(attached.Events); len(got) != 0 {
				t.Fatalf("subscriber retained evidence beyond its byte budget: %v", got)
			}
		})
	}
}

func TestNonReplayablePayloadsDoNotConsumeReplayBudget(t *testing.T) {
	if got := (SegmentProgressed{Progress: Progress{Activity: strings.Repeat("x", 1024)}}).retainedBytes(); got != 0 {
		t.Fatalf("SegmentProgressed retention charge = %d, want 0", got)
	}
	delta, err := newReasoningItemDelta(strings.Repeat("x", 1024))
	if err != nil {
		t.Fatalf("newReasoningItemDelta: %v", err)
	}
	if got := (ItemChanged{Delta: delta}).retainedBytes(); got != 0 {
		t.Fatalf("ItemChanged retention charge = %d, want 0", got)
	}
}
