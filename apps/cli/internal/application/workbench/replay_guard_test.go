package workbench

import (
	"testing"
	"time"

	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

func TestStorePersistsRunAndResumeReplayOwnership(t *testing.T) {
	directory := t.TempDir()
	store, err := OpenDirectory(directory, Config{})
	if err != nil {
		t.Fatal(err)
	}
	startGuard := protectedReplayGuard(t, "runtime-a", time.Now().UTC().Add(time.Hour))
	cancelGuard := protectedReplayGuard(t, "runtime-a", startGuard.Until().Add(time.Minute))
	start := authoring.StartRun{
		CommandID: "cli_88888888888888888888888888888888", SessionID: "ses_1",
		Message: authoring.Message{Text: "persist guards"}, Options: authoring.RunOptions{},
	}
	if stagePendingRunErr := store.StagePendingRun(PendingRun{
		State: PendingRunQueued, Command: start,
		Replay: authoring.UnprotectedGuard(), CancelReplay: authoring.UnprotectedGuard(),
	}); stagePendingRunErr != nil {
		t.Fatal(stagePendingRunErr)
	}
	if markPendingRunDispatchingErr := store.MarkPendingRunDispatching(start.SessionID, start.CommandID, startGuard, nil); markPendingRunDispatchingErr != nil {
		t.Fatal(markPendingRunDispatchingErr)
	}
	cancelID, err := store.MarkPendingRunCanceling(start.SessionID, start.CommandID, cancelGuard)
	if err != nil {
		t.Fatal(err)
	}
	resumeGuard := protectedReplayGuard(t, "runtime-a", cancelGuard.Until().Add(time.Minute))
	approval := conversation.Approval{
		RunID: "run_2", ItemID: "approval_1", Title: "Proceed?",
		Tool: &conversation.ToolCall{Kind: protocol.ToolCategoryShell, Name: "shell", Status: conversation.ToolRunning},
	}
	resume := PendingResume{
		Command: conversation.ResumeRun{
			CommandID: "cli_99999999999999999999999999999999", RunID: approval.RunID,
			Answers: []conversation.InterruptAnswer{{
				ItemID: approval.ItemID, Answer: conversation.ApprovalAnswer{Decision: protocol.ApprovalDeny},
			}},
		},
		Interrupts: []conversation.Interrupt{approval}, Replay: resumeGuard,
	}
	if stagePendingResumeErr := store.StagePendingResume("ses_2", resume, nil); stagePendingResumeErr != nil {
		t.Fatal(stagePendingResumeErr)
	}

	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := OpenDirectory(directory, Config{})
	if err != nil {
		t.Fatal(err)
	}
	runs := reopened.PendingRuns(start.SessionID)
	if len(runs) != 1 || runs[0].Replay != startGuard || runs[0].CancelReplay != cancelGuard ||
		runs[0].CancelCommandID != cancelID {
		t.Fatalf("reopened run ownership = %+v", runs)
	}
	pendingResume, found := reopened.PendingResume("ses_2")
	if !found || pendingResume.Replay != resumeGuard {
		t.Fatalf("reopened resume ownership = %+v, found %t", pendingResume, found)
	}
}

func protectedReplayGuard(t *testing.T, namespace string, until time.Time) authoring.ReplayGuard {
	t.Helper()
	guard, err := authoring.NewProtectedGuard(namespace, until)
	if err != nil {
		t.Fatal(err)
	}
	return guard
}

func queuedPendingRun(command authoring.StartRun) PendingRun {
	return PendingRun{
		State: PendingRunQueued, Command: command,
		Replay: authoring.UnprotectedGuard(), CancelReplay: authoring.UnprotectedGuard(),
	}
}
