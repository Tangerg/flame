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
	home := testsupport.TempDirectory(t)
	t.Setenv("FLAME_HOME", home)
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
	invoke := func(method delivery.Name, params any, key string) protocol.OperationResult {
		return runtime.endpoint.Invoke(ctx, method, params, protocol.CallMetadata{RequestMeta: meta, IdempotencyKey: key})
	}
	withRefusedReceipt := func(call func() protocol.OperationResult) {
		t.Helper()
		questionCrashDatabase(t, home, `CREATE TRIGGER refuse_goal_command_receipt
			BEFORE UPDATE OF payload ON idempotency_records
			BEGIN SELECT RAISE(ABORT, 'Goal command receipt refused'); END`)
		defer questionCrashDatabase(t, home, "DROP TRIGGER refuse_goal_command_receipt")
		if result := call(); result.Failure == nil {
			t.Fatal("Goal command succeeded despite a refused receipt")
		}
	}
	const original = "Keep working on the original objective."
	start := func() protocol.OperationResult {
		return invoke("goals.start", protocol.StartGoalRequest{SessionID: session.ID, Objective: original}, "start-objective")
	}
	assertNoGoal := func() {
		t.Helper()
		current, err := api.GetGoal(ctx, request)
		if err != nil || current != nil {
			t.Fatalf("current Goal = %+v, %v, want none", current, err)
		}
	}
	withRefusedReceipt(start)
	assertNoGoal()
	startResult := start()
	started, ok := startResult.Value.(*protocol.Goal)
	if startResult.Failure != nil || !ok || started.Status != protocol.GoalActive {
		t.Fatalf("start Goal = %+v, %v", started, startResult.Failure)
	}
	waitForSignal(t, model.firstCallStarted, "original Goal Model call")
	runs, err := api.ListRuns(ctx, protocol.ListRunsRequest{SessionID: session.ID})
	if err != nil || len(runs.Data) != 1 {
		t.Fatalf("list Runs = %+v, %v, want the original Goal Run", runs, err)
	}
	originalRunID := runs.Data[0].ID

	update := func(objective, key string) protocol.OperationResult {
		return invoke("goals.update", protocol.UpdateGoalRequest{
			SessionID: session.ID, Objective: objective,
		}, key)
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
	if err := api.StopGoal(ctx, request); !errors.Is(err, protocol.ErrPluginUnavailable) {
		t.Fatalf("stop with unavailable author = %v, want plugin_unavailable", err)
	}
	assertOriginalDrive()
	runtime.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: author.ID, Enabled: true}, "restore-author")

	const edited = "Continue on the replacement objective."
	edit := func() protocol.OperationResult { return update(edited, "edit-objective") }
	withRefusedReceipt(edit)
	assertOriginalDrive()
	result := edit()
	changed, ok := result.Value.(*protocol.Goal)
	if result.Failure != nil || !ok || changed.Status != protocol.GoalActive || changed.Objective != edited || changed.Used.Runs != 0 {
		t.Fatalf("edit Goal = %+v, %v, want active replacement with no prior Runs", changed, result.Failure)
	}
	waitForSignal(t, model.resumedCallStarted, "replacement Goal Model call")
	currentRun, err := api.GetRun(ctx, protocol.GetRunRequest{RunID: originalRunID})
	if err != nil || currentRun.Status != protocol.RunStatusFinished {
		t.Fatalf("original Run when replacement starts = %+v, %v, want finished", currentRun, err)
	}
	stop := func() protocol.OperationResult { return invoke("goals.stop", request, "stop-objective") }
	withRefusedReceipt(stop)
	standing, err := api.GetGoal(ctx, request)
	if err != nil || standing == nil || standing.Status != protocol.GoalActive || standing.Used.Runs != 0 {
		t.Fatalf("Goal after refused Stop receipt = %+v, %v, want the replacement running", standing, err)
	}
	if result := stop(); result.Failure != nil || result.Value != (struct{}{}) {
		t.Fatalf("stop acknowledgement = %+v, %v", result.Value, result.Failure)
	}
	assertPaused := func() {
		t.Helper()
		current, err := api.GetGoal(ctx, request)
		if err != nil || current == nil || current.Status != protocol.GoalPaused || current.Objective != edited ||
			current.Reason == nil || current.Reason.Code != protocol.GoalReasonStoppedByUser || current.Used.Runs != 1 {
			t.Fatalf("settled Goal = %+v, %v, want paused replacement with exactly one Run", current, err)
		}
	}
	clear := func() protocol.OperationResult { return invoke("goals.clear", request, "clear-objective") }
	resume := func() protocol.OperationResult { return invoke("goals.resume", request, "resume-objective") }
	assertPaused()
	withRefusedReceipt(clear)
	assertPaused()
	withRefusedReceipt(resume)
	assertPaused()
	resumeResult := resume()
	resumed, ok := resumeResult.Value.(*protocol.Goal)
	if resumeResult.Failure != nil || !ok || resumed.Status != protocol.GoalActive || resumed.Used.Runs != 1 {
		t.Fatalf("resume Goal = %+v, %v, want active replacement with one settled Run", resumed, resumeResult.Failure)
	}
	if result := stop(); result.Failure != nil || result.Value != (struct{}{}) {
		t.Fatalf("replayed stop acknowledgement = %+v, %v", result.Value, result.Failure)
	}
	standing, err = api.GetGoal(ctx, request)
	if err != nil || standing == nil || standing.Status != protocol.GoalActive {
		t.Fatalf("replayed Stop changed the resumed Goal = %+v, %v", standing, err)
	}
	if result := clear(); result.Failure != nil {
		t.Fatal(result.Failure)
	}
	if result := edit(); result.Failure != nil {
		t.Fatal(result.Failure)
	}
	assertNoGoal()
}
