package runs

import (
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/domain/session/plan"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func TestRetentionChargeTracksEveryVariableReplayPayload(t *testing.T) {
	const growth = 32 << 10
	largeText := strings.Repeat("x", growth)
	largeResult := tool.StringResult(largeText)
	canceled := run.OutcomeCanceled

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
