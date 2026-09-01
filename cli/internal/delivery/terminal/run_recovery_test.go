package terminal

import (
	"errors"
	"testing"
	"time"

	runworkflow "github.com/Tangerg/flame/cli/internal/application/agent/run"
	"github.com/Tangerg/flame/cli/internal/application/agent/workbench"
	"github.com/Tangerg/flame/cli/internal/domain/agent"
	"github.com/Tangerg/flame/cli/internal/runtimefixture"
)

func TestPrepareSessionKeepsExpiredSteerAsARecoveryIssue(t *testing.T) {
	stateDirectory := t.TempDir()
	store, err := openSessionWorkbench(stateDirectory)
	if err != nil {
		t.Fatal(err)
	}
	stagedAt := time.Now().UTC().Add(-time.Hour)
	guard := protectedCommandReplayGuard(
		t,
		terminalTestReplayNamespace,
		time.Now().UTC().Add(-time.Minute),
	)
	command := agent.SteerRun{
		CommandID: "cli_55555555555555555555555555555555",
		RunID:     "run_expired",
		SegmentID: "seg_expired",
		Message:   agent.Message{Text: "preserve me"},
	}
	pending, err := workbench.NewPendingSteer("ses_expired", command, stagedAt, guard)
	if err != nil {
		t.Fatal(err)
	}
	source := agent.Message{Text: "/steer preserve me"}
	if err := store.SaveDraft(pending.SessionID(), source); err != nil {
		t.Fatal(err)
	}
	if err := store.StagePendingSteer(pending, source); err != nil {
		t.Fatal(err)
	}

	workspace := t.TempDir()
	profile := steerReplayTestProfile(t, workspace)
	prepared, err := prepareSession(t.Context(), Config{
		Runtime:        runtimefixture.New(),
		RuntimeProfile: &profile,
		Workspace:      workspace,
		StateDirectory: stateDirectory,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !errors.Is(prepared.recoveryIssues.steer, runworkflow.ErrSteerReplayUnavailable) {
		t.Fatalf("steer recovery issue = %v, want ErrSteerReplayUnavailable", prepared.recoveryIssues.steer)
	}
	if durable, found := prepared.workbench.PendingSteer(pending.SessionID()); !found ||
		!durable.Command().Equal(pending.Command()) {
		t.Fatalf("expired pending steer = %+v, found %t", durable, found)
	}
}
