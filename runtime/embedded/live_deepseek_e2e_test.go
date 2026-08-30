package embedded_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/embedded"
	"github.com/Tangerg/flame/runtime/protocol"
)

const (
	liveDeepSeekEnvironment        = "FLAME_LIVE_DEEPSEEK"
	liveConfigDirectoryEnvironment = "FLAME_LIVE_CONFIG_DIR"
	liveGoalMarker                 = "LIVE_GOAL_PLAN_OK"
	liveSteerMarker                = "LIVE_STEER_APPLIED"
	liveOriginMarker               = "LIVE_CONTEXT_ORIGIN_7D3A91"
)

func TestLiveDeepSeekGoalAndPlan(t *testing.T) {
	fixture := newLiveDeepSeekFixture(t, 3*time.Minute)
	session := fixture.createSession(t, "Live DeepSeek Goal and Plan E2E")
	maxRuns, maxSteps := 2, 12
	goal, err := fixture.runtime.StartGoal(fixture.ctx, protocol.StartGoalRequest{
		SessionID: session.ID,
		Objective: "Use set_plan exactly once to create one completed step whose description is " +
			liveGoalMarker + ". Then call report_goal_outcome with outcome completed. " +
			"Do not ask the user, run shell commands, edit files, or perform any other work.",
		Budget: &protocol.GoalBudget{MaxRuns: &maxRuns, MaxSteps: &maxSteps},
	}, embedded.CommandOptions{})
	if err != nil {
		t.Fatal(err)
	}
	lastGoal := goal
	for {
		current, getErr := fixture.runtime.GetGoal(
			fixture.ctx,
			protocol.GoalRequest{SessionID: session.ID},
			embedded.CallOptions{},
		)
		if getErr != nil {
			t.Fatal(getErr)
		}
		if current == nil {
			break
		}
		lastGoal = current
		if current.Status == protocol.GoalPaused || current.Status == protocol.GoalBlocked {
			t.Fatalf("Goal stopped before completion: status=%s reason=%+v usage=%+v", current.Status, current.Reason, current.Used)
		}
		select {
		case <-fixture.ctx.Done():
			t.Fatal(fixture.ctx.Err())
		case <-time.After(100 * time.Millisecond):
		}
	}
	plan, err := fixture.runtime.GetPlan(
		fixture.ctx,
		protocol.GetPlanRequest{SessionID: session.ID},
		embedded.CallOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if plan.State == nil || len(plan.State.Steps) != 1 ||
		plan.State.Steps[0].Description != liveGoalMarker ||
		plan.State.Steps[0].Status != protocol.PlanStatusCompleted {
		t.Fatalf("Plan = %+v, want one completed %q step", plan, liveGoalMarker)
	}
	runs, err := fixture.runtime.ListRuns(
		fixture.ctx,
		protocol.ListRunsRequest{SessionID: session.ID},
		embedded.CallOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs.Data) != 1 || runs.Data[0].Status != protocol.RunStatusFinished ||
		runs.Data[0].Outcome == nil || runs.Data[0].Outcome.Type != protocol.OutcomeCompleted {
		t.Fatalf("Goal Runs = %+v, want one completed Run", runs.Data)
	}
	assertDeepSeekRun(t, &runs.Data[0])
	t.Logf("Goal settled after %d steps for $%.6f; final observable state was %s", lastGoal.Used.Steps, lastGoal.Used.CostUSD, lastGoal.Status)
}

func TestLiveDeepSeekSteerAtToolBoundary(t *testing.T) {
	fixture := newLiveDeepSeekFixture(t, 3*time.Minute)
	if _, err := fixture.runtime.SetApprovalMode(
		fixture.ctx,
		protocol.SetApprovalModeRequest{Mode: protocol.ApprovalModeYolo},
		embedded.CommandOptions{},
	); err != nil {
		t.Fatal(err)
	}
	session := fixture.createSession(t, "Live DeepSeek Steer E2E")
	maxSteps := 8
	started, events, err := fixture.runtime.StartRun(fixture.ctx, protocol.StartRunRequest{
		SessionID: session.ID,
		Input: []protocol.ContentBlock{{
			Type: protocol.ContentBlockText,
			Text: "Call shell exactly once with command `sleep 3` and a concise description. " +
				"After it finishes, answer exactly ORIGINAL_RESPONSE. Do not use any other tool. " +
				"Obey a later user steer instruction over this prompt.",
		}},
		Limits: &protocol.RunLimits{MaxSteps: &maxSteps},
	}, embedded.RunCommandOptions{})
	if err != nil {
		t.Fatal(err)
	}
	shellStarted, steerSent := false, false
	for event, eventErr := range events {
		if eventErr != nil {
			t.Fatal(eventErr)
		}
		if event.Event.Type != protocol.StreamItemStarted || event.Event.Item == nil ||
			event.Event.Item.Type != protocol.ItemTypeToolCall || event.Event.Item.Tool == nil ||
			event.Event.Item.Tool.Name != "shell" {
			continue
		}
		shellStarted = true
		if steerSent {
			continue
		}
		if err := fixture.runtime.SteerRun(fixture.ctx, protocol.SteerRunRequest{
			RunID:             started.RunID,
			ExpectedSegmentID: started.SegmentID,
			Input: []protocol.ContentBlock{{
				Type: protocol.ContentBlockText,
				Text: "Override the earlier final-answer instruction. After the running shell settles, " +
					"reply with exactly " + liveSteerMarker + " and nothing else.",
			}},
		}, embedded.CommandOptions{}); err != nil {
			t.Fatal(err)
		}
		steerSent = true
	}
	if !shellStarted || !steerSent {
		t.Fatalf("Steer precondition missing: shellStarted=%t steerSent=%t", shellStarted, steerSent)
	}
	run := fixture.getCompletedRun(t, started.RunID)
	if got := strings.TrimSpace(fixture.finalAnswer(t, started.RunID)); got != liveSteerMarker {
		t.Fatalf("final answer = %q, want %q", got, liveSteerMarker)
	}
	assertDeepSeekRun(t, run)
}

func TestLiveDeepSeekLongContextCompaction(t *testing.T) {
	fixture := newLiveDeepSeekFixture(t, 5*time.Minute)
	session := fixture.createSession(t, "Live DeepSeek Long Context Compaction E2E")
	first := fixture.runTurn(t, session.ID,
		"Remember the exact origin marker "+liveOriginMarker+" for a later recall check. Reply with exactly ACK_01.")
	if got := strings.TrimSpace(first.finalText); got != "ACK_01" {
		t.Fatalf("first answer = %q", got)
	}
	compactionSeen := first.compactionSeen
	for turn := 2; turn <= 12; turn++ {
		prompt := fmt.Sprintf(
			"This is context turn %02d. Preserve all earlier facts, especially the original marker. The local fact for this turn is FACT_%02d_%s. Reply with exactly ACK_%02d.",
			turn, turn, strings.Repeat(string(rune('A'+turn%26)), 512), turn,
		)
		current := fixture.runTurn(t, session.ID, prompt)
		if got, want := strings.TrimSpace(current.finalText), fmt.Sprintf("ACK_%02d", turn); got != want {
			t.Fatalf("turn %d answer = %q, want %q", turn, got, want)
		}
		compactionSeen = compactionSeen || current.compactionSeen
	}
	final := fixture.runTurn(t, session.ID,
		"Recall the exact origin marker from the first turn. Reply with that marker only, with no explanation or formatting.")
	compactionSeen = compactionSeen || final.compactionSeen
	if got := strings.TrimSpace(final.finalText); got != liveOriginMarker {
		t.Fatalf("post-compaction recall = %q, want %q", got, liveOriginMarker)
	}
	limit := 200
	items, err := fixture.runtime.ListItems(fixture.ctx, protocol.ListItemsRequest{
		Scope:     protocol.ItemListScope{Type: protocol.ItemScopeSession, SessionID: session.ID},
		PageQuery: protocol.PageQuery{Limit: &limit},
	}, embedded.CallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	compactionItems := 0
	for _, item := range items.Data {
		if item.Type == protocol.ItemTypeCompaction {
			compactionItems++
		}
	}
	if !compactionSeen || compactionItems != 1 {
		t.Fatalf("observable compactions = stream:%t transcript:%d, want one", compactionSeen, compactionItems)
	}
	usage, err := fixture.runtime.GetSessionUsage(
		fixture.ctx,
		protocol.SessionUsageRequest{SessionID: session.ID},
		embedded.CallOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}
	cost := 0.0
	if usage.CostUSD != nil {
		cost = *usage.CostUSD
	}
	t.Logf("13 real turns compacted once: input=%d cost=$%.6f", usage.InputTokens, cost)
}

type liveDeepSeekFixture struct {
	ctx       context.Context
	runtime   *embedded.Runtime
	workspace string
}

func newLiveDeepSeekFixture(t *testing.T, timeout time.Duration) liveDeepSeekFixture {
	t.Helper()
	if os.Getenv(liveDeepSeekEnvironment) != "1" {
		t.Skipf("set %s=1 to run paid live DeepSeek E2E", liveDeepSeekEnvironment)
	}
	configDirectory := os.Getenv(liveConfigDirectoryEnvironment)
	if configDirectory == "" {
		configDirectory = filepath.Join("..", "config")
	}
	configDirectory, err := filepath.Abs(configDirectory)
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), timeout)
	t.Cleanup(cancel)
	runtime, err := embedded.Open(ctx, embedded.Config{
		DataDirectory:        filepath.Join(t.TempDir(), "runtime"),
		DefaultWorkspacePath: workspace,
		UserHomePath:         t.TempDir(),
		ConfigDirectories:    []string{configDirectory},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		var closeErr error
		for range 3 {
			closeErr = runtime.Close()
			if closeErr == nil || errors.Is(closeErr, embedded.ErrClosed) {
				return
			}
		}
		t.Errorf("close live Runtime: %v", closeErr)
	})
	return liveDeepSeekFixture{ctx: ctx, runtime: runtime, workspace: workspace}
}

func (f liveDeepSeekFixture) createSession(t *testing.T, title string) *protocol.Session {
	t.Helper()
	session, err := f.runtime.CreateSession(f.ctx, protocol.CreateSessionRequest{
		Workspace: &protocol.WorkspaceRef{Path: f.workspace},
		Title:     title,
	}, embedded.CommandOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return session
}

type liveTurn struct {
	finalText      string
	compactionSeen bool
}

func (f liveDeepSeekFixture) runTurn(t *testing.T, sessionID, prompt string) liveTurn {
	t.Helper()
	maxSteps := 4
	started, events, err := f.runtime.StartRun(f.ctx, protocol.StartRunRequest{
		SessionID: sessionID,
		Input:     []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: prompt}},
		Limits:    &protocol.RunLimits{MaxSteps: &maxSteps},
	}, embedded.RunCommandOptions{})
	if err != nil {
		t.Fatal(err)
	}
	compactionSeen := false
	for event, eventErr := range events {
		if eventErr != nil {
			t.Fatal(eventErr)
		}
		if event.Event.Type == protocol.StreamItemCompleted && event.Event.Item != nil &&
			event.Event.Item.Type == protocol.ItemTypeCompaction {
			compactionSeen = true
		}
	}
	f.getCompletedRun(t, started.RunID)
	return liveTurn{finalText: f.finalAnswer(t, started.RunID), compactionSeen: compactionSeen}
}

func (f liveDeepSeekFixture) getCompletedRun(t *testing.T, runID string) *protocol.RunRef {
	t.Helper()
	run, err := f.runtime.GetRun(f.ctx, protocol.GetRunRequest{RunID: runID}, embedded.CallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != protocol.RunStatusFinished || run.Outcome == nil || run.Outcome.Type != protocol.OutcomeCompleted {
		t.Fatalf("Run %s = %+v, want completed", runID, run)
	}
	return run
}

func (f liveDeepSeekFixture) finalAnswer(t *testing.T, runID string) string {
	t.Helper()
	limit := 100
	items, err := f.runtime.ListItems(f.ctx, protocol.ListItemsRequest{
		Scope:     protocol.ItemListScope{Type: protocol.ItemScopeRun, RunID: runID},
		PageQuery: protocol.PageQuery{Limit: &limit},
	}, embedded.CallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for index := len(items.Data) - 1; index >= 0; index-- {
		item := items.Data[index]
		if item.Type != protocol.ItemTypeAgentMessage || item.Phase != protocol.MessagePhaseFinalAnswer {
			continue
		}
		var text strings.Builder
		for _, block := range item.Content {
			if block.Type == protocol.ContentBlockText {
				text.WriteString(block.Text)
			}
		}
		return text.String()
	}
	t.Fatalf("Run %s has no final AgentMessage", runID)
	return ""
}

func assertDeepSeekRun(t *testing.T, run *protocol.RunRef) {
	t.Helper()
	if run.Provider != "deepseek" || run.Model == "" || run.Metrics.Usage == nil ||
		run.Metrics.Usage.CostUSD == nil || *run.Metrics.Usage.CostUSD <= 0 {
		t.Fatalf("Run did not use metered DeepSeek: %+v", run)
	}
}
