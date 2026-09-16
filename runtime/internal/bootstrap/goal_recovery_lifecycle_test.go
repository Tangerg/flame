package bootstrap

import (
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/automation/goal"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

// TestGoalOwnedRunLostOnRestartIsChargedOnceAndHandedBackToTheUser walks the
// product lifecycle the unit tests only cover in halves: a Goal was driving a
// Run when the process died.
//
// Startup owes the user three things here, and they belong to three different
// owners. Run recovery must terminalize the abandoned Run as lost. The Goal
// ledger must charge that Run exactly once, because a Goal's budget is the only
// thing standing between an autonomous drive and an unbounded spend. And the
// Goal must end in a state the user can act on rather than active with a Run
// that no longer exists.
//
// The pause here is the accounting's, not the driver's: a charged Run that did
// not complete pauses its Goal and names the outcome, so the user is told the
// Run was lost rather than merely that the process restarted. The driver's own
// runtimeRestarted pause is for the Goal that had no Run to charge, which
// TestActiveGoalWithNoRunInFlightPausesAsRestarted covers.
//
// Ordering is the part no single-owner test can see: the Goal charge reads the
// durable Run row the recovery commit has just written, and the driver's sweep
// then compare-and-swaps against the revision that charge moved.
func TestGoalOwnedRunLostOnRestartIsChargedOnceAndHandedBackToTheUser(t *testing.T) {
	cfg := runtimeConfigWithRequiredDeps(t)
	ctx := t.Context()
	const (
		sessionID     = "ses_goal_lost"
		runID         = "run_goal_lost"
		incarnationID = "incarnation_goal_lost"
	)
	createdAt := time.Date(2026, 9, 2, 9, 0, 0, 0, time.UTC)
	selection, err := modelref.New("anthropic", "claude-test")
	if err != nil {
		t.Fatal(err)
	}
	// The driver clears a Goal whose Session lost its model selection, so the
	// Session must carry one for this to reach the pause it is meant to prove.
	if insertErr := cfg.Stores.Sessions.Insert(ctx, testsupport.MustRestoreSession(session.Snapshot{
		ID: sessionID, Workspace: testsupport.MustWorkspace(t.TempDir()),
		Selection: selection, CreatedAt: createdAt, UpdatedAt: createdAt,
	})); insertErr != nil {
		t.Fatalf("insert Session: %v", insertErr)
	}

	budget, err := goal.NewBudget(goal.BudgetLimits{MaxRuns: testsupport.Pointer(3)})
	if err != nil {
		t.Fatal(err)
	}
	active, err := goal.New(sessionID, "drive until done", selection, budget, run.Capabilities{}, incarnationID, createdAt)
	if err != nil {
		t.Fatal(err)
	}
	unwritten, err := goal.Unwritten(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	opening, err := goal.NewReplacement(unwritten.Version(), active)
	if err != nil {
		t.Fatal(err)
	}
	if applied, saveErr := cfg.Stores.Goals.Save(ctx, opening); saveErr != nil || !applied {
		t.Fatalf("save opening Goal = (%t, %v)", applied, saveErr)
	}

	// The crash window: admitted under the Goal's incarnation, never terminalized.
	if admitErr := cfg.Stores.Runs.Admit(ctx, run.Draft{
		RunID: runID, SessionID: sessionID, SegmentID: "seg_goal_lost",
		ModelSelection: selection, GoalIncarnationID: incarnationID, CreatedAt: createdAt,
	}); admitErr != nil {
		t.Fatalf("admit Goal-owned Run: %v", admitErr)
	}

	host, err := assemble(ctx, cfg, newRuntimeLifetime(t.Context(), cfg.Resources), buildToolEnvironment)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	t.Cleanup(func() { _ = host.Close() })

	storedRuns, err := cfg.Stores.Runs.ListRuns(ctx, sessionID)
	if err != nil || len(storedRuns) != 1 {
		t.Fatalf("runs after recovery = (%+v, %v), want one", storedRuns, err)
	}
	failure, failed := storedRuns[0].Failure()
	if !storedRuns[0].State().IsTerminal() || !failed || failure.Kind != run.FailureLost {
		t.Fatalf("recovered Run = %+v, want a run_lost terminal", storedRuns[0].Snapshot())
	}

	recovered, exists, err := readBootstrapGoal(ctx, cfg.Stores.Goals, sessionID)
	if err != nil || !exists {
		t.Fatalf("Goal after recovery = (%t, %v), want the Goal to survive its Run", exists, err)
	}
	if recovered.Used().Runs != 1 {
		t.Fatalf("Goal usage after recovery = %+v, want the lost Run charged once", recovered.Used())
	}
	if recovered.Status() != goal.StatusPaused ||
		recovered.Reason().Code() != goal.ReasonRunNotCompleted ||
		recovered.Reason().Detail() != string(run.OutcomeLost) {
		t.Fatalf("Goal after recovery = %s/%s(%q), want paused/runNotCompleted naming the lost outcome",
			recovered.Status(), recovered.Reason().Code(), recovered.Reason().Detail())
	}

	// Recovery is re-entered on every boot, and a Goal's budget is cumulative.
	// A restart loop that re-charged the same Run would exhaust the budget
	// without the agent doing any work, so the second startup must find nothing
	// left to reconcile rather than the same Run again.
	if closeErr := host.Close(); closeErr != nil {
		t.Fatalf("close first Runtime: %v", closeErr)
	}
	secondHost, err := assemble(ctx, cfg, newRuntimeLifetime(t.Context(), cfg.Resources), buildToolEnvironment)
	if err != nil {
		t.Fatalf("second assemble: %v", err)
	}
	t.Cleanup(func() { _ = secondHost.Close() })

	rebooted, exists, err := readBootstrapGoal(ctx, cfg.Stores.Goals, sessionID)
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

	// The Session is the user's again: nothing holds its single-writer slot.
	if admitErr := cfg.Stores.Runs.Admit(ctx, run.Draft{
		RunID: "run_after_recovery", SessionID: sessionID, SegmentID: "seg_after_recovery",
		ModelSelection: selection, CreatedAt: createdAt.Add(time.Hour),
	}); admitErr != nil {
		t.Fatalf("admit after recovery = %v, want the Session slot freed", admitErr)
	}
}

// TestActiveGoalWithNoRunInFlightPausesAsRestarted is the other half of the
// distinction. A Goal that died between Runs has nothing to charge, so the
// accounting never speaks and the driver's own sweep owns the pause. Naming the
// restart rather than a Run outcome is the whole point: no Run failed here.
func TestActiveGoalWithNoRunInFlightPausesAsRestarted(t *testing.T) {
	cfg := runtimeConfigWithRequiredDeps(t)
	ctx := t.Context()
	const (
		sessionID     = "ses_goal_idle"
		incarnationID = "incarnation_goal_idle"
	)
	createdAt := time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC)
	selection, err := modelref.New("anthropic", "claude-test")
	if err != nil {
		t.Fatal(err)
	}
	if insertErr := cfg.Stores.Sessions.Insert(ctx, testsupport.MustRestoreSession(session.Snapshot{
		ID: sessionID, Workspace: testsupport.MustWorkspace(t.TempDir()),
		Selection: selection, CreatedAt: createdAt, UpdatedAt: createdAt,
	})); insertErr != nil {
		t.Fatalf("insert Session: %v", insertErr)
	}
	budget, err := goal.NewBudget(goal.BudgetLimits{MaxRuns: testsupport.Pointer(3)})
	if err != nil {
		t.Fatal(err)
	}
	active, err := goal.New(sessionID, "drive until done", selection, budget, run.Capabilities{}, incarnationID, createdAt)
	if err != nil {
		t.Fatal(err)
	}
	unwritten, err := goal.Unwritten(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	opening, err := goal.NewReplacement(unwritten.Version(), active)
	if err != nil {
		t.Fatal(err)
	}
	if applied, saveErr := cfg.Stores.Goals.Save(ctx, opening); saveErr != nil || !applied {
		t.Fatalf("save opening Goal = (%t, %v)", applied, saveErr)
	}

	host, err := assemble(ctx, cfg, newRuntimeLifetime(t.Context(), cfg.Resources), buildToolEnvironment)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	t.Cleanup(func() { _ = host.Close() })

	recovered, exists, err := readBootstrapGoal(ctx, cfg.Stores.Goals, sessionID)
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
