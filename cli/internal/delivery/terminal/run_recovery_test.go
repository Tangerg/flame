package terminal

import (
	"errors"
	"testing"
	"time"

	runworkflow "github.com/Tangerg/flame/cli/internal/application/agent/run"
	"github.com/Tangerg/flame/cli/internal/application/agent/workbench"
	"github.com/Tangerg/flame/cli/internal/domain/agent"
	"github.com/Tangerg/flame/cli/internal/domain/commandreplay"
	"github.com/Tangerg/flame/cli/internal/runtimefixture"
	"github.com/Tangerg/flame/runtime/protocol"
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

func TestPrepareSessionMergesInitialPromptAfterConfirmedRollbackRecovery(t *testing.T) {
	runtime := runtimefixture.New()
	workspace := t.TempDir()
	created, err := runtime.CreateSession(t.Context(), agent.CreateSession{Workspace: workspace})
	if err != nil {
		t.Fatal(err)
	}
	stateDirectory := t.TempDir()
	store, err := openSessionWorkbench(stateDirectory)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveDraft(created.ID, agent.Message{Text: "existing draft"}); err != nil {
		t.Fatal(err)
	}
	pending := workbench.PendingSessionRollback{
		Phase:          workbench.SessionRollbackPrepared,
		CommandID:      "cli_66666666666666666666666666666666",
		SessionID:      created.ID,
		Scope:          protocol.RestoreHistory,
		BeforeRevision: created.Revision,
		BeforeRunIDs:   []string{"run_1"},
		OpeningText:    "restored opening",
		StagedAt:       time.Now().UTC(),
		Replay:         commandreplay.UnprotectedGuard(),
	}
	if err := store.StageSessionRollback(pending); err != nil {
		t.Fatal(err)
	}
	if err := store.ConfirmSessionRollback(created.ID, pending.CommandID); err != nil {
		t.Fatal(err)
	}

	prepared, err := prepareSession(t.Context(), Config{
		Runtime:        runtime,
		SessionID:      created.ID,
		Workspace:      workspace,
		StateDirectory: stateDirectory,
		InitialPrompt:  "from argv",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := agent.Message{Text: "restored opening\n\nexisting draft\n\nfrom argv"}
	if !prepared.draft.Equal(want) {
		t.Fatalf("prepared draft = %+v, want %+v", prepared.draft, want)
	}
	if prepared.rollbackRecovery == nil || !prepared.rollbackRecovery.Draft.Equal(want) {
		t.Fatalf("rollback recovery = %+v, want merged draft", prepared.rollbackRecovery)
	}
	if durable, found := prepared.workbench.Draft(created.ID); !found || !durable.Equal(want) {
		t.Fatalf("durable draft = %+v, found %t", durable, found)
	}
}
