package sessions

import (
	"testing"
	"time"

	"github.com/Tangerg/scope/core/chat"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func TestTerminalPlanOwnsProjectionAndDerivesGoalRun(t *testing.T) {
	createdAt := time.Date(2026, 9, 5, 1, 2, 3, 0, time.UTC)
	finishedAt := createdAt.Add(time.Minute)
	parked := testsupport.MustRestoreRun(run.Snapshot{
		ID: "run_1", SessionID: "ses_1", State: run.Waiting,
		GoalIncarnationID: "lease_1", CreatedAt: createdAt,
	})
	terminal, err := parked.CancelWaiting("stopped", finishedAt, run.MessageMarkAt(1))
	if err != nil {
		t.Fatalf("cancel waiting Run: %v", err)
	}
	runningItem := testsupport.MustRestoreItem(testsupport.ItemInput{
		ID: "item_1", RunID: "run_1", SessionID: "ses_1",
		Kind: transcript.ToolCall, Status: transcript.ItemRunning, OccurredAt: createdAt,
	})
	incompleteItem, err := runningItem.AbandonToolCall(nil, finishedAt)
	if err != nil {
		t.Fatalf("abandon ToolCall: %v", err)
	}
	runs := []run.Replacement{testsupport.MustRunReplacement(parked, testsupport.DecidedRun(terminal))}
	items := []transcript.Item{incompleteItem}
	messages := []chat.Message{chat.NewUserMessage(chat.NewTextPart("closed"))}

	plan, err := NewTerminalPlan(runs, items, messages, "member_1")
	if err != nil {
		t.Fatalf("NewTerminalPlan: %v", err)
	}

	runs[0] = run.Replacement{}
	items[0] = transcript.Item{}
	messages[0].Parts[0].Text = "changed input"
	ownedRuns := plan.Runs()
	ownedItems := plan.Items()
	ownedMessages := plan.Messages()
	ownedRuns[0] = run.Replacement{}
	ownedItems[0] = transcript.Item{}
	ownedMessages[0].Parts[0].Text = "changed accessor"

	gotRuns := plan.Runs()
	gotItems := plan.Items()
	gotMessages := plan.Messages()
	if gotRuns[0].State().ID() != "run_1" || gotItems[0].ID() != "item_1" ||
		gotMessages[0].Parts[0].Text != "closed" {
		t.Fatalf(
			"terminal ownership = run:%q item:%q message:%q",
			gotRuns[0].State().ID(), gotItems[0].ID(), gotMessages[0].Parts[0].Text,
		)
	}
}
