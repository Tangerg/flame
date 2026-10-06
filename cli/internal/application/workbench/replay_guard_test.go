package workbench

import (
	"testing"
	"time"

	"github.com/Tangerg/flame/cli/internal/domain/authoring/prompt"
	"github.com/Tangerg/flame/cli/internal/domain/authoring/replay"
	"github.com/Tangerg/flame/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/runtime/protocol"
)

func TestStorePersistsRunAndResumeReplayOwnership(t *testing.T) {
	directory := t.TempDir()
	store, err := OpenDirectory(directory, Config{})
	if err != nil {
		t.Fatal(err)
	}
	startGuard := protectedReplayGuard(t, "runtime-a", time.Now().UTC().Add(time.Hour))
	cancelGuard := protectedReplayGuard(t, "runtime-a", startGuard.Until().Add(time.Minute))
	start := prompt.StartRun{
		CommandID: "cli_88888888888888888888888888888888", SessionID: "ses_1",
		Message: prompt.Message{Text: "persist guards"}, Options: prompt.RunOptions{},
	}
	if stagePendingRunErr := store.StagePendingRun(PendingRun{
		State: PendingRunQueued, Command: start,
		Replay: replay.UnprotectedGuard(), CancelReplay: replay.UnprotectedGuard(),
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
		Tool: &conversation.ToolCall{Kind: conversation.ToolShell, Name: "shell", Status: conversation.ToolRunning},
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

func protectedReplayGuard(t *testing.T, namespace string, until time.Time) replay.Guard {
	t.Helper()
	guard, err := replay.NewProtectedGuard(namespace, until)
	if err != nil {
		t.Fatal(err)
	}
	return guard
}

func queuedPendingRun(command prompt.StartRun) PendingRun {
	return PendingRun{
		State: PendingRunQueued, Command: command,
		Replay: replay.UnprotectedGuard(), CancelReplay: replay.UnprotectedGuard(),
	}
}
