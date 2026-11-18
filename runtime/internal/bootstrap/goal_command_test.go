package bootstrap

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func TestGoalCommandsSettleTheDriveAfterCommit(t *testing.T) {
	t.Setenv("FLAME_HOME", testsupport.TempDirectory(t))
	workspace := t.TempDir()
	model := newLifecycleModel()
	host, api := openProtocolRuntime(t, model)
	runtime := &poMCPRuntime{t: t, host: host, api: api, endpoint: host.Endpoint()}
	t.Cleanup(runtime.close)
	author := poMCPActivate(runtime, filepath.Join(firstPartyPackages, "goals"))
	ctx := protocolLifecycleContext(t.Context())
	meta, _ := delivery.RequestMetaFrom(ctx)
	session, err := api.CreateSession(ctx, protocol.CreateSessionRequest{
		Workspace: &protocol.WorkspaceRef{Path: workspace}, Title: "running Goal",
	})
	if err != nil {
		t.Fatal(err)
	}
	request := protocol.GoalRequest{SessionID: session.ID}
	const original = "Keep working on the original objective."
	started, err := api.StartGoal(ctx, protocol.StartGoalRequest{SessionID: session.ID, Objective: original})
	if err != nil || started.Status != protocol.GoalActive {
		t.Fatalf("start Goal = %+v, %v", started, err)
	}
	waitForSignal(t, model.firstCallStarted, "original Goal Model call")
	runs, err := api.ListRuns(ctx, protocol.ListRunsRequest{SessionID: session.ID})
	if err != nil || len(runs.Data) != 1 {
		t.Fatalf("list Runs = %+v, %v, want the original Goal Run", runs, err)
	}
	originalRunID := runs.Data[0].ID

	update := func(objective, key string) protocol.OperationResult {
		return runtime.endpoint.Invoke(ctx, delivery.Name("goals.update"), protocol.UpdateGoalRequest{
			SessionID: session.ID, Objective: objective,
		}, protocol.CallMetadata{RequestMeta: meta, IdempotencyKey: key})
	}
	assertOriginalDrive := func() {
		t.Helper()
		currentRun, err := api.GetRun(ctx, protocol.GetRunRequest{RunID: originalRunID})
		if err != nil || currentRun.Status != protocol.RunStatusRunning {
			t.Fatalf("original Run = %+v, %v, want running", currentRun, err)
		}
		current, err := api.GetGoal(ctx, request)
		if err != nil || current == nil || current.Status != protocol.GoalActive || current.Objective != original {
			t.Fatalf("original Goal = %+v, %v, want original active objective", current, err)
		}
	}
	if refusal := update("\u2003", "invalid-objective").Failure; !errors.Is(refusal, protocol.ErrInvalidParams) {
		t.Fatalf("blank objective = %v, want invalid_params", refusal)
	}
	assertOriginalDrive()

	runtime.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: author.ID, Enabled: false}, "withdraw-author")
	if _, err := api.StopGoal(ctx, request); !errors.Is(err, protocol.ErrPluginUnavailable) {
		t.Fatalf("stop with unavailable author = %v, want plugin_unavailable", err)
	}
	assertOriginalDrive()
	runtime.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: author.ID, Enabled: true}, "restore-author")

	const edited = "Continue on the replacement objective."
	result := update(edited, "edit-objective")
	changed, ok := result.Value.(*protocol.Goal)
	if result.Failure != nil || !ok || changed.Status != protocol.GoalActive || changed.Objective != edited || changed.Used.Runs != 0 {
		t.Fatalf("edit Goal = %+v, %v, want active replacement with no prior Runs", changed, result.Failure)
	}
	waitForSignal(t, model.resumedCallStarted, "replacement Goal Model call")
	currentRun, err := api.GetRun(ctx, protocol.GetRunRequest{RunID: originalRunID})
	if err != nil || currentRun.Status != protocol.RunStatusFinished {
		t.Fatalf("original Run when replacement starts = %+v, %v, want finished", currentRun, err)
	}
	stopped, err := api.StopGoal(ctx, request)
	if err != nil || stopped.Status != protocol.GoalPaused || stopped.Reason == nil || stopped.Reason.Code != protocol.GoalReasonStoppedByUser || stopped.Used.Runs != 1 {
		t.Fatalf("stop Goal = %+v, %v, want stopped by user with one settled Run", stopped, err)
	}
	current, err := api.GetGoal(ctx, request)
	if err != nil || current == nil || current.Status != protocol.GoalPaused || current.Objective != edited || current.Used.Runs != 1 {
		t.Fatalf("settled Goal = %+v, %v, want paused replacement with exactly one Run", current, err)
	}
}
