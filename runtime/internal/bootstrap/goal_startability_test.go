package bootstrap

import (
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func TestGoalContinuesAfterEarlierManualQuestionIsCanceled(t *testing.T) {
	t.Setenv("FLAME_HOME", testsupport.TempDirectory(t))
	model := newLifecycleModel()
	host, api := openProtocolRuntime(t, model)
	t.Cleanup(func() {
		if err := host.Close(); err != nil {
			t.Error(err)
		}
	})
	ctx := protocolLifecycleContext(t.Context())
	session, err := api.CreateSession(ctx, protocol.CreateSessionRequest{
		Workspace: &protocol.WorkspaceRef{Path: t.TempDir()}, Title: "manual question before Goal",
	})
	if err != nil {
		t.Fatal(err)
	}
	manual, events, err := api.StartRun(ctx, protocol.StartRunRequest{
		SessionID: session.ID, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "Ask before continuing."}},
	})
	if err != nil {
		t.Fatal(err)
	}
	waitForSignal(t, model.firstCallStarted, "manual model call")
	close(model.releaseFirstCall)
	waitForRunEvents(t, collectRunEvents(events), "manual question boundary")
	waiting, err := api.GetRun(ctx, protocol.GetRunRequest{RunID: manual.RunID})
	if err != nil || waiting.Status != protocol.RunStatusWaiting {
		t.Fatalf("manual Run = %+v, %v; want waiting", waiting, err)
	}
	started, err := api.StartGoal(ctx, protocol.StartGoalRequest{
		SessionID: session.ID, Objective: "Continue after the earlier manual question is settled.",
		Provider: "anthropic", Model: "claude-test",
	})
	if err != nil || started.Status != protocol.GoalActive {
		t.Fatalf("Goal start = %+v, %v", started, err)
	}
	if _, err := api.CancelRun(ctx, protocol.CancelRunRequest{RunID: manual.RunID, Reason: "continue with the Goal"}); err != nil {
		t.Fatal(err)
	}
	waitForSignal(t, model.resumedCallStarted, "first Goal-owned model call")
	page, err := api.ListRuns(ctx, protocol.ListRunsRequest{SessionID: session.ID})
	if err != nil || len(page.Data) != 2 {
		t.Fatalf("Runs after manual cancellation = %+v, %v; want the manual and Goal Runs", page, err)
	}
	stopped, err := api.StopGoal(ctx, protocol.GoalRequest{SessionID: session.ID})
	if err != nil || stopped.Status != protocol.GoalPaused {
		t.Fatalf("Goal stop = %+v, %v; want paused", stopped, err)
	}
}
