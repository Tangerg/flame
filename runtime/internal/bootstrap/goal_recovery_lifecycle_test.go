package bootstrap

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/goalworkflow"
	"github.com/Tangerg/flame/runtime/internal/adapter/integration/pluginservices"
	"github.com/Tangerg/flame/runtime/internal/testsupport/persistencefixture"

	flamemodel "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	"github.com/Tangerg/flame/runtime/internal/domain/automation/goal"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func TestGoalOwnedRunLostOnRestartIsChargedOnceAndHandedBackToTheUser(t *testing.T) {
	cfg := runtimeConfigWithRequiredDeps(t)
	ctx := t.Context()
	const (
		sessionID     = "ses_goal_lost"
		runID         = "run_goal_lost"
		incarnationID = "incarnation_goal_lost"
	)
	createdAt := time.Date(2026, 9, 2, 9, 0, 0, 0, time.UTC)
	selection, err := flamemodel.New("anthropic", "claude-test")
	if err != nil {
		t.Fatal(err)
	}
	if insertErr := cfg.stores.Sessions.Insert(ctx, testsupport.MustRestoreSession(session.Snapshot{
		ID: testsupport.MustSessionID(sessionID), Workspace: testsupport.MustWorkspace(t.TempDir()),
		Selection: selection, CreatedAt: createdAt, UpdatedAt: createdAt,
	})); insertErr != nil {
		t.Fatalf("insert Session: %v", insertErr)
	}

	producer := poMCPOpen(t, cfg)
	poMCPActivate(producer, filepath.Join(firstPartyPackages, "goals"))
	calls := producer.host.application.delivery.PluginCalls.(*pluginservices.CommandCalls)
	decision, err := calls.DecideGoal(ctx, extension.ContributionSource{}, goalworkflow.Request{Signal: goalworkflow.Start, Objective: "drive until done"})
	if err != nil || decision.Next == nil {
		t.Fatalf("original author did not issue the objective: %v", err)
	}
	active, err := (goal.Snapshot{SessionID: testsupport.MustSessionID(sessionID), CreatedAt: createdAt, ModelSelection: selection}).Publish(*decision.Next, decision.Source, testsupport.MustIncarnation(incarnationID), createdAt)
	if err != nil {
		t.Fatal(err)
	}
	unwritten, err := goal.Unwritten(testsupport.MustSessionID(sessionID))
	if err != nil {
		t.Fatal(err)
	}
	opening, err := goal.NewReplacement(unwritten.Version(), active)
	if err != nil {
		t.Fatal(err)
	}
	if applied, saveErr := cfg.stores.Goals.Save(ctx, opening); saveErr != nil || !applied {
		t.Fatalf("save opening Goal = (%t, %v)", applied, saveErr)
	}

	if admitErr := cfg.stores.Runs.Admit(ctx, run.Draft{
		RunID: testsupport.MustRunID(runID), SessionID: testsupport.MustSessionID(sessionID), SegmentID: testsupport.MustSegmentID("seg_goal_lost"),
		ModelSelection: selection, GoalIncarnationID: testsupport.MustIncarnation(incarnationID), CreatedAt: createdAt,
	}); admitErr != nil {
		t.Fatalf("admit Goal-owned Run: %v", admitErr)
	}

	host := reopenPluginRuntime(t, producer, &cfg).host

	if err := host.application.recoverUncertainExecutions(ctx); err != nil {
		t.Fatalf("recover uncertain executions: %v", err)
	}

	storedRuns, err := cfg.stores.Runs.ListRuns(ctx, testsupport.MustSessionID(sessionID))
	if err != nil || len(storedRuns) != 1 {
		t.Fatalf("runs after recovery = (%+v, %v), want one", storedRuns, err)
	}
	if _, lost := storedRuns[0].Loss(); !storedRuns[0].State().IsTerminal() || !lost {
		t.Fatalf("recovered Run = %+v, want a run_lost terminal", storedRuns[0].Snapshot())
	}

	recovered, exists, err := readBootstrapGoal(ctx, cfg.stores.Goals, sessionID)
	if err != nil || !exists {
		t.Fatalf("Goal after recovery = (%t, %v), want the Goal to survive its Run", exists, err)
	}
	if recovered.Used().Runs != 1 {
		t.Fatalf("Goal usage after recovery = %+v, want the lost Run charged once", recovered.Used())
	}
	if recovered.Status() != goal.StatusPaused ||
		recovered.Reason().Code() != goal.ReasonRunNotCompleted ||
		!lostRunReason(recovered.Reason()) {
		t.Fatalf("Goal after recovery = %s/%+v, want paused/runNotCompleted naming the lost outcome",
			recovered.Status(), recovered.Reason())
	}

	if closeErr := host.Close(); closeErr != nil {
		t.Fatalf("close first Runtime: %v", closeErr)
	}
	if err := cfg.stores.Close(); err != nil {
		t.Fatalf("close first storage owner: %v", err)
	}
	cfg.stores, err = persistence.Open(ctx, persistence.Config{DataDirectory: cfg.stores.DataDirectory, ValidateDirectory: persistencefixture.UnleasedDirectory})
	if err != nil {
		t.Fatalf("reopen storage: %v", err)
	}
	t.Cleanup(func() {
		if err := cfg.stores.Close(); err != nil {
			t.Error(err)
		}
	})
	cfg.providerRegistry = cfg.stores.Providers
	secondHost, err := assemble(ctx, cfg, newRuntimeLifetime(t.Context(), cfg.resources), buildToolEnvironment)
	if err != nil {
		t.Fatalf("second assemble: %v", err)
	}
	t.Cleanup(func() { _ = secondHost.Close() })

	if err := secondHost.application.recoverUncertainExecutions(ctx); err != nil {
		t.Fatalf("recover uncertain executions: %v", err)
	}

	rebooted, exists, err := readBootstrapGoal(ctx, cfg.stores.Goals, sessionID)
	if err != nil || !exists {
		t.Fatalf("Goal after second boot = (%t, %v)", exists, err)
	}
	if rebooted.Used().Runs != 1 {
		t.Fatalf("Goal usage after second boot = %+v, want the same single charge", rebooted.Used())
	}
	if rebooted.Status() != goal.StatusPaused ||
		rebooted.Reason().Code() != goal.ReasonRunNotCompleted {
		t.Fatalf("Goal after second boot = %s/%s, want the paused Goal left alone",
			rebooted.Status(), rebooted.Reason().Code())
	}

	if admitErr := cfg.stores.Runs.Admit(ctx, run.Draft{
		RunID: testsupport.MustRunID("run_after_recovery"), SessionID: testsupport.MustSessionID(sessionID), SegmentID: testsupport.MustSegmentID("seg_after_recovery"),
		ModelSelection: selection, CreatedAt: createdAt.Add(time.Hour),
	}); admitErr != nil {
		t.Fatalf("admit after recovery = %v, want the Session slot freed", admitErr)
	}
}

func TestActiveGoalWithNoRunInFlightPausesAsRestarted(t *testing.T) {
	cfg := runtimeConfigWithRequiredDeps(t)
	ctx := t.Context()
	const (
		sessionID     = "ses_goal_idle"
		incarnationID = "incarnation_goal_idle"
	)
	createdAt := time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC)
	selection, err := flamemodel.New("anthropic", "claude-test")
	if err != nil {
		t.Fatal(err)
	}
	if insertErr := cfg.stores.Sessions.Insert(ctx, testsupport.MustRestoreSession(session.Snapshot{
		ID: testsupport.MustSessionID(sessionID), Workspace: testsupport.MustWorkspace(t.TempDir()),
		Selection: selection, CreatedAt: createdAt, UpdatedAt: createdAt,
	})); insertErr != nil {
		t.Fatalf("insert Session: %v", insertErr)
	}
	producer := poMCPOpen(t, cfg)
	poMCPActivate(producer, filepath.Join(firstPartyPackages, "goals"))
	calls := producer.host.application.delivery.PluginCalls.(*pluginservices.CommandCalls)
	decision, err := calls.DecideGoal(ctx, extension.ContributionSource{}, goalworkflow.Request{Signal: goalworkflow.Start, Objective: "drive until done"})
	if err != nil || decision.Next == nil {
		t.Fatalf("original author did not issue the objective: %v", err)
	}
	active, err := (goal.Snapshot{SessionID: testsupport.MustSessionID(sessionID), CreatedAt: createdAt, ModelSelection: selection}).Publish(*decision.Next, decision.Source, testsupport.MustIncarnation(incarnationID), createdAt)
	if err != nil {
		t.Fatal(err)
	}
	unwritten, err := goal.Unwritten(testsupport.MustSessionID(sessionID))
	if err != nil {
		t.Fatal(err)
	}
	opening, err := goal.NewReplacement(unwritten.Version(), active)
	if err != nil {
		t.Fatal(err)
	}
	if applied, saveErr := cfg.stores.Goals.Save(ctx, opening); saveErr != nil || !applied {
		t.Fatalf("save opening Goal = (%t, %v)", applied, saveErr)
	}

	host := reopenPluginRuntime(t, producer, &cfg).host

	if err := host.application.recoverUncertainExecutions(ctx); err != nil {
		t.Fatalf("recover uncertain executions: %v", err)
	}

	recovered, exists, err := readBootstrapGoal(ctx, cfg.stores.Goals, sessionID)
	if err != nil || !exists {
		t.Fatalf("Goal after restart = (%t, %v), want the Goal to survive", exists, err)
	}
	if recovered.Used().Runs != 0 {
		t.Fatalf("Goal usage after restart = %+v, want nothing charged", recovered.Used())
	}
	if recovered.Status() != goal.StatusPaused ||
		recovered.Reason().Code() != goal.ReasonRuntimeRestarted {
		t.Fatalf("Goal after restart = %s/%s, want paused/runtimeRestarted",
			recovered.Status(), recovered.Reason().Code())
	}
}

func lostRunReason(reason goal.Reason) bool {
	outcome, ok := reason.RunOutcome()
	return ok && outcome == run.OutcomeLost
}
