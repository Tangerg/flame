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
	terminal, err := parked.CancelWaiting("stopped", finishedAt, 1)
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
	runs := []run.Replacement{testsupport.MustRunReplacement(parked, terminal)}
	items := []transcript.Item{incompleteItem}
	messages := []chat.Message{chat.NewUserMessage(chat.NewTextPart("closed"))}

	plan, err := NewTerminalPlan(runs, items, messages, "member_1")
	if err != nil {
		t.Fatalf("NewTerminalPlan: %v", err)
	}
	if plan.ConsumesClaimedResume() {
		t.Fatal("ordinary terminal plan consumes a claimed Resume")
	}
	goalRun := plan.GoalRun()
	if goalRun == nil || goalRun.SessionID != "ses_1" || goalRun.IncarnationID != "lease_1" ||
		goalRun.RunID != "run_1" || goalRun.Outcome != run.OutcomeCanceled ||
		!goalRun.CompletedAt.Equal(finishedAt) {
		t.Fatalf("derived Goal Run = %+v", goalRun)
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
	goalRun.SessionID = "ses_changed"

	gotRuns := plan.Runs()
	gotItems := plan.Items()
	gotMessages := plan.Messages()
	gotGoalRun := plan.GoalRun()
	if gotRuns[0].State().ID() != "run_1" || gotItems[0].ID() != "item_1" ||
		gotMessages[0].Parts[0].Text != "closed" || gotGoalRun.SessionID != "ses_1" {
		t.Fatalf(
			"terminal ownership = run:%q item:%q message:%q goal:%q",
			gotRuns[0].State().ID(), gotItems[0].ID(), gotMessages[0].Parts[0].Text, gotGoalRun.SessionID,
		)
	}
	if err := plan.Validate(); err != nil {
		t.Fatalf("Validate after caller mutations: %v", err)
	}
}

func TestClaimedResumeTerminalPlanRequiresLostRun(t *testing.T) {
	createdAt := time.Date(2026, 9, 5, 2, 3, 4, 0, time.UTC)
	finishedAt := createdAt.Add(time.Minute)
	parked := testsupport.MustRestoreRun(run.Snapshot{
		ID: "run_1", SessionID: "ses_1", State: run.Waiting, CreatedAt: createdAt,
	})
	canceled, err := parked.CancelWaiting("stopped", finishedAt, 0)
	if err != nil {
		t.Fatalf("cancel waiting Run: %v", err)
	}
	if _, err := NewClaimedResumeTerminalPlan(
		[]run.Replacement{testsupport.MustRunReplacement(parked, canceled)}, nil, nil, "member_1",
	); err == nil {
		t.Fatal("claimed Resume terminal plan accepted a canceled Run")
	}
	lost, err := parked.RecoverLost(run.Failure{Kind: run.FailureLost}, finishedAt, 0)
	if err != nil {
		t.Fatalf("recover lost Run: %v", err)
	}
	plan, err := NewClaimedResumeTerminalPlan(
		[]run.Replacement{testsupport.MustRunReplacement(parked, lost)}, nil, nil, "member_1",
	)
	if err != nil {
		t.Fatalf("NewClaimedResumeTerminalPlan: %v", err)
	}
	if !plan.ConsumesClaimedResume() {
		t.Fatal("claimed Resume terminal plan lost its consumption mode")
	}
}
