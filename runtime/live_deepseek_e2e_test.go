package runtime_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	flameruntime "github.com/Tangerg/flame/runtime"
	"github.com/Tangerg/flame/runtime/protocol"
)

const (
	liveDeepSeekEnvironment        = "FLAME_LIVE_DEEPSEEK"
	liveConfigDirectoryEnvironment = "FLAME_LIVE_CONFIG_DIR"
	liveGoalMarker                 = "LIVE_GOAL_PLAN_OK"
	liveSteerMarker                = "LIVE_STEER_APPLIED"
	liveHITLMarker                 = "LIVE_HITL_RESUMED"
	liveCrashRecoveryMarker        = "LIVE_CRASH_RECOVERY_OK"
	liveOriginMarker               = "LIVE_CONTEXT_ORIGIN_7D3A91"
	liveCrashHelperEnvironment     = "FLAME_LIVE_CRASH_HELPER"
	liveCrashDataEnvironment       = "FLAME_LIVE_CRASH_DATA_DIR"
	liveCrashHomeEnvironment       = "FLAME_LIVE_CRASH_HOME"
	liveCrashWorkspaceEnvironment  = "FLAME_LIVE_CRASH_WORKSPACE"
	liveCrashStateEnvironment      = "FLAME_LIVE_CRASH_STATE"
	liveCrashToolMarkerEnvironment = "FLAME_LIVE_CRASH_TOOL_MARKER"
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
	}, flameruntime.CommandOptions{})
	if err != nil {
		t.Fatal(err)
	}
	lastGoal := goal
	for {
		current, getErr := fixture.runtime.GetGoal(
			fixture.ctx,
			protocol.GoalRequest{SessionID: session.ID},
			flameruntime.CallOptions{},
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
		flameruntime.CallOptions{},
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
		flameruntime.CallOptions{},
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
		flameruntime.CommandOptions{},
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
	}, flameruntime.RunCommandOptions{})
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
		}, flameruntime.CommandOptions{}); err != nil {
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

func TestLiveDeepSeekQuestionSurvivesRuntimeRestart(t *testing.T) {
	fixture := newLiveDeepSeekFixture(t, 4*time.Minute)
	session := fixture.createSession(t, "Live DeepSeek HITL Restart E2E")
	requestMeta := protocol.RequestMeta{ClientCapabilities: &protocol.ClientCapabilities{
		InterruptTypes: []protocol.InterruptType{protocol.InterruptQuestion},
	}}
	maxSteps := 6
	started, events, err := fixture.runtime.StartRun(fixture.ctx, protocol.StartRunRequest{
		SessionID: session.ID,
		Input: []protocol.ContentBlock{{
			Type: protocol.ContentBlockText,
			Text: "Call ask_user exactly once with one free-text question asking for a release codename. " +
				"After the tool returns, reply with exactly " + liveHITLMarker + " and nothing else. " +
				"Do not use another tool and do not answer before asking the question.",
		}},
		Limits: &protocol.RunLimits{MaxSteps: &maxSteps},
	}, flameruntime.RunCommandOptions{RequestMeta: requestMeta})
	if err != nil {
		t.Fatal(err)
	}
	for _, eventErr := range events {
		if eventErr != nil {
			t.Fatal(eventErr)
		}
	}
	waiting, err := fixture.runtime.GetRun(
		fixture.ctx, protocol.GetRunRequest{RunID: started.RunID}, flameruntime.CallOptions{RequestMeta: requestMeta},
	)
	if err != nil {
		t.Fatal(err)
	}
	if waiting.Status != protocol.RunStatusWaiting {
		t.Fatalf("Run before restart = %+v, want waiting", waiting)
	}
	_ = fixture.pendingQuestion(t, session.ID, started.RunID, requestMeta)

	fixture.restart(t)
	pending := fixture.pendingQuestion(t, session.ID, started.RunID, requestMeta)
	resumed, resumedEvents, err := fixture.runtime.ResumeRun(fixture.ctx, protocol.ResumeRunRequest{
		RunID: started.RunID,
		Responses: []protocol.InterruptResponse{{
			ItemID: pending.Interrupts[0].ItemID,
			Response: protocol.InterruptResponseValue{
				Type:    protocol.InterruptResponseAnswer,
				Answers: [][]string{{"phoenix"}},
			},
		}},
	}, flameruntime.RunCommandOptions{RequestMeta: requestMeta})
	if err != nil {
		t.Fatal(err)
	}
	if resumed.RunID != started.RunID || resumed.SegmentID == started.SegmentID {
		t.Fatalf("Resume response = %+v, want same Run and a new Segment", resumed)
	}
	for _, eventErr := range resumedEvents {
		if eventErr != nil {
			t.Fatal(eventErr)
		}
	}
	run := fixture.getCompletedRun(t, started.RunID)
	if got := strings.TrimSpace(fixture.finalAnswer(t, started.RunID)); got != liveHITLMarker {
		t.Fatalf("post-restart final answer = %q, want %q", got, liveHITLMarker)
	}
	assertDeepSeekRun(t, run)
}

// TestLiveDeepSeekRecoversKilledLongRunningTool crosses the process boundary
// that an in-process Close cannot model: the helper is killed after its shell
// command has actually started, leaving a durable running Run and Tool
// invocation behind. A fresh Runtime must recover that tree as lost and make
// the Session immediately usable again.
func TestLiveDeepSeekRecoversKilledLongRunningTool(t *testing.T) {
	if os.Getenv(liveDeepSeekEnvironment) != "1" {
		t.Skipf("set %s=1 to run paid live DeepSeek E2E", liveDeepSeekEnvironment)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 4*time.Minute)
	t.Cleanup(cancel)

	configDirectory := resolveLiveConfigDirectory(t)
	workspace, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	dataDirectory := filepath.Join(t.TempDir(), "runtime")
	userHome := t.TempDir()
	coordinationDirectory := t.TempDir()
	statePath := filepath.Join(coordinationDirectory, "run.json")
	toolMarkerPath := filepath.Join(coordinationDirectory, "tool-started")

	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(ctx, executable, "-test.run=^TestLiveDeepSeekCrashHelper$", "-test.v")
	command.Env = append(os.Environ(),
		liveCrashHelperEnvironment+"=1",
		liveCrashDataEnvironment+"="+dataDirectory,
		liveCrashHomeEnvironment+"="+userHome,
		liveCrashWorkspaceEnvironment+"="+workspace,
		liveCrashStateEnvironment+"="+statePath,
		liveCrashToolMarkerEnvironment+"="+toolMarkerPath,
		liveConfigDirectoryEnvironment+"="+configDirectory,
	)
	command.Stdout = os.Stderr
	command.Stderr = os.Stderr
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	helperDone := make(chan error, 1)
	go func() { helperDone <- command.Wait() }()
	t.Cleanup(func() { _ = command.Process.Kill() })

	stateContent := waitForLiveCrashFile(t, ctx, statePath, helperDone)
	var state liveCrashState
	if err := json.Unmarshal(stateContent, &state); err != nil || state.SessionID == "" || state.RunID == "" {
		t.Fatalf("decode live crash state %q: %v", stateContent, err)
	}
	waitForLiveCrashFile(t, ctx, toolMarkerPath, helperDone)
	if err := command.Process.Kill(); err != nil {
		t.Fatalf("kill live crash helper: %v", err)
	}
	waitErr := <-helperDone
	if waitErr == nil {
		t.Fatal("live crash helper exited normally; process kill was not observed")
	}

	runtime, err := flameruntime.Open(ctx, flameruntime.Config{
		DataDirectory:        dataDirectory,
		DefaultWorkspacePath: workspace,
		UserHomePath:         userHome,
		ConfigDirectories:    []string{configDirectory},
	})
	if err != nil {
		t.Fatalf("open Runtime after killed helper: %v", err)
	}
	t.Cleanup(func() {
		if closeErr := runtime.Close(); closeErr != nil && !errors.Is(closeErr, flameruntime.ErrClosed) {
			t.Errorf("close recovered Runtime: %v", closeErr)
		}
	})
	fixture := liveDeepSeekFixture{ctx: ctx, runtime: runtime, workspace: workspace}

	recovered, err := runtime.GetRun(ctx, protocol.GetRunRequest{RunID: state.RunID}, flameruntime.CallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Status != protocol.RunStatusFinished || recovered.Outcome == nil || recovered.Outcome.Type != protocol.OutcomeLost {
		t.Fatalf("recovered Run = %+v, want finished(lost)", recovered)
	}
	assertDeepSeekRun(t, recovered)
	assertRecoveredShellIncomplete(t, runtime, ctx, state.RunID)

	followUp := fixture.runTurn(t, state.SessionID,
		"The previous process stopped while a shell tool was running. Reply with exactly "+liveCrashRecoveryMarker+" and nothing else. Do not use a tool.")
	if got := strings.TrimSpace(followUp.finalText); got != liveCrashRecoveryMarker {
		t.Fatalf("post-recovery answer = %q, want %q", got, liveCrashRecoveryMarker)
	}
	assertDeepSeekRun(t, followUp.run)
}

// TestLiveDeepSeekCrashHelper is started only by the parent recovery test. It
// intentionally never performs cleanup: the parent terminates this process
// once the exact shell command has created its marker.
func TestLiveDeepSeekCrashHelper(t *testing.T) {
	if os.Getenv(liveCrashHelperEnvironment) != "1" {
		t.Skip("live crash helper is subprocess-only")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()
	configDirectory := resolveLiveConfigDirectory(t)
	runtime, err := flameruntime.Open(ctx, flameruntime.Config{
		DataDirectory:        requiredLiveCrashPath(t, liveCrashDataEnvironment),
		DefaultWorkspacePath: requiredLiveCrashPath(t, liveCrashWorkspaceEnvironment),
		UserHomePath:         requiredLiveCrashPath(t, liveCrashHomeEnvironment),
		ConfigDirectories:    []string{configDirectory},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SetApprovalMode(ctx, protocol.SetApprovalModeRequest{
		Mode: protocol.ApprovalModeYolo,
	}, flameruntime.CommandOptions{}); err != nil {
		t.Fatal(err)
	}
	fixture := liveDeepSeekFixture{
		ctx:       ctx,
		runtime:   runtime,
		workspace: requiredLiveCrashPath(t, liveCrashWorkspaceEnvironment),
	}
	session := fixture.createSession(t, "Live DeepSeek crash recovery helper")
	toolMarkerPath := requiredLiveCrashPath(t, liveCrashToolMarkerEnvironment)
	maxSteps := 6
	started, events, err := runtime.StartRun(ctx, protocol.StartRunRequest{
		SessionID: session.ID,
		Input: []protocol.ContentBlock{{
			Type: protocol.ContentBlockText,
			Text: "Call shell exactly once with this exact command: `touch " + toolMarkerPath + " && sleep 8`. " +
				"After it finishes, reply exactly UNREACHABLE_HELPER_RESPONSE. Do not use another tool.",
		}},
		Limits: &protocol.RunLimits{MaxSteps: &maxSteps},
	}, flameruntime.RunCommandOptions{})
	if err != nil {
		t.Fatal(err)
	}
	writeLiveCrashState(t, requiredLiveCrashPath(t, liveCrashStateEnvironment), liveCrashState{
		SessionID: session.ID,
		RunID:     started.RunID,
	})
	for _, eventErr := range events {
		if eventErr != nil {
			t.Fatal(eventErr)
		}
	}
	t.Fatal("live crash helper Run settled before the parent killed it")
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
	}, flameruntime.CallOptions{})
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
		flameruntime.CallOptions{},
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
	ctx               context.Context
	runtime           *flameruntime.Runtime
	workspace         string
	dataDirectory     string
	userHome          string
	configDirectories []string
}

func newLiveDeepSeekFixture(t *testing.T, timeout time.Duration) *liveDeepSeekFixture {
	t.Helper()
	if os.Getenv(liveDeepSeekEnvironment) != "1" {
		t.Skipf("set %s=1 to run paid live DeepSeek E2E", liveDeepSeekEnvironment)
	}
	configDirectory := resolveLiveConfigDirectory(t)
	workspace, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), timeout)
	t.Cleanup(cancel)
	fixture := &liveDeepSeekFixture{
		ctx:               ctx,
		workspace:         workspace,
		dataDirectory:     filepath.Join(t.TempDir(), "runtime"),
		userHome:          t.TempDir(),
		configDirectories: []string{configDirectory},
	}
	runtime, err := flameruntime.Open(ctx, fixture.config())
	if err != nil {
		t.Fatal(err)
	}
	fixture.runtime = runtime
	t.Cleanup(func() {
		var closeErr error
		for range 3 {
			closeErr = fixture.runtime.Close()
			if closeErr == nil || errors.Is(closeErr, flameruntime.ErrClosed) {
				return
			}
		}
		t.Errorf("close live Runtime: %v", closeErr)
	})
	return fixture
}

type liveCrashState struct {
	SessionID string `json:"sessionId"`
	RunID     string `json:"runId"`
}

func resolveLiveConfigDirectory(t *testing.T) string {
	t.Helper()
	directory := os.Getenv(liveConfigDirectoryEnvironment)
	if directory == "" {
		directory = filepath.Join("..", "config")
	}
	resolved, err := filepath.Abs(directory)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

func requiredLiveCrashPath(t *testing.T, name string) string {
	t.Helper()
	value := os.Getenv(name)
	if value == "" || !filepath.IsAbs(value) {
		t.Fatalf("%s must be a non-empty absolute path", name)
	}
	return filepath.Clean(value)
}

func writeLiveCrashState(t *testing.T, path string, state liveCrashState) {
	t.Helper()
	content, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, content, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(temporary, path); err != nil {
		t.Fatal(err)
	}
}

func waitForLiveCrashFile(
	t *testing.T,
	ctx context.Context,
	path string,
	helperDone <-chan error,
) []byte {
	t.Helper()
	for {
		content, err := os.ReadFile(path)
		if err == nil {
			return content
		} else if !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
		select {
		case <-ctx.Done():
			t.Fatalf("waiting for live crash file %s: %v", filepath.Base(path), ctx.Err())
		case err := <-helperDone:
			t.Fatalf("live crash helper exited before publishing %s: %v", filepath.Base(path), err)
		case <-time.After(25 * time.Millisecond):
		}
	}
}

func assertRecoveredShellIncomplete(t *testing.T, runtime *flameruntime.Runtime, ctx context.Context, runID string) {
	t.Helper()
	limit := 100
	items, err := runtime.ListItems(ctx, protocol.ListItemsRequest{
		Scope:     protocol.ItemListScope{Type: protocol.ItemScopeRun, RunID: runID},
		PageQuery: protocol.PageQuery{Limit: &limit},
	}, flameruntime.CallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range items.Data {
		if item.Type == protocol.ItemTypeToolCall && item.Tool != nil && item.Tool.Name == "shell" {
			if item.Status != protocol.ItemStatusIncomplete {
				t.Fatalf("recovered shell Item = %+v, want incomplete", item)
			}
			return
		}
	}
	t.Fatalf("recovered Run %s has no shell Item", runID)
}

func (f *liveDeepSeekFixture) config() flameruntime.Config {
	return flameruntime.Config{
		DataDirectory:        f.dataDirectory,
		DefaultWorkspacePath: f.workspace,
		UserHomePath:         f.userHome,
		ConfigDirectories:    f.configDirectories,
	}
}

func (f *liveDeepSeekFixture) restart(t *testing.T) {
	t.Helper()
	if err := f.runtime.Close(); err != nil {
		t.Fatal(err)
	}
	runtime, err := flameruntime.Open(f.ctx, f.config())
	if err != nil {
		t.Fatal(err)
	}
	f.runtime = runtime
}

func (f *liveDeepSeekFixture) pendingQuestion(
	t *testing.T,
	sessionID, runID string,
	requestMeta protocol.RequestMeta,
) protocol.PendingInterruptSet {
	t.Helper()
	interrupts, err := f.runtime.ListInterrupts(f.ctx, protocol.ListInterruptsRequest{
		SessionID: sessionID,
	}, flameruntime.CallOptions{RequestMeta: requestMeta})
	if err != nil {
		t.Fatal(err)
	}
	if len(interrupts.Data) != 1 || interrupts.Data[0].RootRunID != runID ||
		len(interrupts.Data[0].Interrupts) != 1 ||
		interrupts.Data[0].Interrupts[0].Type != protocol.InterruptQuestion ||
		interrupts.Data[0].Interrupts[0].Payload == nil ||
		interrupts.Data[0].Interrupts[0].Payload.Question == nil {
		t.Fatalf("pending interrupts = %+v, want one question for Run %s", interrupts.Data, runID)
	}
	return interrupts.Data[0]
}

func (f liveDeepSeekFixture) createSession(t *testing.T, title string) *protocol.Session {
	t.Helper()
	session, err := f.runtime.CreateSession(f.ctx, protocol.CreateSessionRequest{
		Workspace: &protocol.WorkspaceRef{Path: f.workspace},
		Title:     title,
	}, flameruntime.CommandOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return session
}

type liveTurn struct {
	finalText      string
	compactionSeen bool
	run            *protocol.RunRef
}

func (f liveDeepSeekFixture) runTurn(t *testing.T, sessionID, prompt string) liveTurn {
	t.Helper()
	maxSteps := 4
	started, events, err := f.runtime.StartRun(f.ctx, protocol.StartRunRequest{
		SessionID: sessionID,
		Input:     []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: prompt}},
		Limits:    &protocol.RunLimits{MaxSteps: &maxSteps},
	}, flameruntime.RunCommandOptions{})
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
	run := f.getCompletedRun(t, started.RunID)
	return liveTurn{finalText: f.finalAnswer(t, started.RunID), compactionSeen: compactionSeen, run: run}
}

func (f liveDeepSeekFixture) getCompletedRun(t *testing.T, runID string) *protocol.RunRef {
	t.Helper()
	run, err := f.runtime.GetRun(f.ctx, protocol.GetRunRequest{RunID: runID}, flameruntime.CallOptions{})
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
	}, flameruntime.CallOptions{})
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
