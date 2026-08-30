package workbench

import (
	"testing"
	"time"

	"github.com/Tangerg/flame/cli/internal/agent"
	"github.com/Tangerg/flame/cli/internal/commandreplay"
)

func TestStorePersistsRunAndResumeReplayOwnership(t *testing.T) {
	directory := t.TempDir()
	store, err := OpenDirectory(directory, Config{})
	if err != nil {
		t.Fatal(err)
	}
	startGuard := protectedReplayGuard(t, "runtime-a", time.Now().UTC().Add(time.Hour))
	cancelGuard := protectedReplayGuard(t, "runtime-a", startGuard.Until().Add(time.Minute))
	start := agent.StartRun{
		CommandID: "cli_88888888888888888888888888888888", SessionID: "ses_1",
		Message: agent.Message{Text: "persist guards"}, Options: agent.RunOptions{Limits: agent.UnlimitedRunLimits()},
	}
	if stagePendingRunErr := store.StagePendingRun(PendingRun{
		State: PendingRunQueued, Command: start,
		Replay: commandreplay.UnprotectedGuard(), CancelReplay: commandreplay.UnprotectedGuard(),
	}); stagePendingRunErr != nil {
		t.Fatal(stagePendingRunErr)
	}
	if markPendingRunDispatchingErr := store.MarkPendingRunDispatching(start.SessionID, start.CommandID, startGuard); markPendingRunDispatchingErr != nil {
		t.Fatal(markPendingRunDispatchingErr)
	}
	cancelID, err := store.MarkPendingRunCanceling(start.SessionID, start.CommandID, cancelGuard)
	if err != nil {
		t.Fatal(err)
	}
	resumeGuard := protectedReplayGuard(t, "runtime-a", cancelGuard.Until().Add(time.Minute))
	approval := agent.Approval{
		RunID: "run_2", ItemID: "approval_1", Title: "Proceed?",
		Tool: &agent.ToolCall{Kind: agent.ToolShell, Name: "shell", Status: agent.ToolRunning},
	}
	resume := PendingResume{
		Command: agent.ResumeRun{
			CommandID: "cli_99999999999999999999999999999999", RunID: approval.RunID,
			Answers: []agent.InterruptAnswer{{
				ItemID: approval.ItemID, Answer: agent.ApprovalAnswer{Decision: agent.ApprovalDeny},
			}},
		},
		Interactions: []agent.Interaction{approval}, Replay: resumeGuard,
	}
	if stagePendingResumeErr := store.StagePendingResume("ses_2", resume); stagePendingResumeErr != nil {
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

func protectedReplayGuard(t *testing.T, namespace string, until time.Time) commandreplay.Guard {
	t.Helper()
	guard, err := commandreplay.NewProtectedGuard(namespace, until)
	if err != nil {
		t.Fatal(err)
	}
	return guard
}

func queuedPendingRun(command agent.StartRun) PendingRun {
	if command.Options.Limits == (agent.RunLimits{}) {
		command.Options.Limits = agent.UnlimitedRunLimits()
	}
	return PendingRun{
		State: PendingRunQueued, Command: command,
		Replay: commandreplay.UnprotectedGuard(), CancelReplay: commandreplay.UnprotectedGuard(),
	}
}
