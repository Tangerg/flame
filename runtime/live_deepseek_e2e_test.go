package runtime_test

import (
	"context"
	"crypto/rand"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/packages/go-sdk/runevent"
	flameruntime "github.com/Tangerg/flame/runtime"
	"github.com/Tangerg/flame/runtime/internal/testsupport"

	framecall "github.com/Tangerg/frame/call"
)

const (
	liveDeepSeekEnvironment        = "FLAME_LIVE_DEEPSEEK"
	liveConfigDirectoryEnvironment = "FLAME_LIVE_CONFIG_DIR"
	liveGoalMarker                 = "LIVE_GOAL_PLAN_OK"
	liveScheduleMarker             = "LIVE_SCHEDULE_RUN_OK"
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

func TestLiveDeepSeekManualScheduleRun(t *testing.T) {
	fixture := newLiveDeepSeekFixture(t, 3*time.Minute)
	creation := fixtureScheduleCreateRequest(t, fixture, testsupport.SchedulesDraft{Enabled: true, Title: "Live DeepSeek Schedule E2E", Instructions: "Reply exactly " + liveScheduleMarker + ". Do not call tools or ask the user anything.", Cwd: fixture.workspace, ModelSelection: &testsupport.SchedulesCreateV2InputModelSelection{}, Cron: "0 0 1 1 *", TimeZone: "UTC"})
	created, err := fixtureScheduleCreate(fixture, creation)
	if err != nil {
		t.Fatal(err)
	}
	if liveScheduleRevision(t, created.Revision) != 1 || created.LastRunAt != nil || created.NextRunAt == nil {
		t.Fatalf("created Schedule = %+v, want pristine revision one", created)
	}

	request := fixtureScheduleRunRequest(t, fixture, created.Id)
	options := protocol.PluginCallOptions{IdempotencyKey: "live-deepseek-manual-schedule"}
	started, err := fixtureScheduleRun(fixture, request, options)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := fixtureScheduleRun(fixture, request, options)
	if err != nil {
		t.Fatal(err)
	}
	if *replayed != *started {
		t.Fatalf("replayed Schedule Run = %+v, want %+v", replayed, started)
	}

	var completed *protocol.RunRef
	for completed == nil {
		current, getErr := fixture.runtime.GetRun(
			fixture.ctx, protocol.GetRunRequest{RunID: started.RunId}, protocol.CallOptions{},
		)
		if getErr != nil {
			t.Fatal(getErr)
		}
		switch current.Status {
		case protocol.RunStatusRunning:
		case protocol.RunStatusFinished:
			if current.Outcome == nil || current.Outcome.Type != protocol.OutcomeCompleted {
				t.Fatalf("manual Schedule Run = %+v, want completed", current)
			}
			completed = current
		default:
			t.Fatalf("manual Schedule Run = %+v, want running or completed", current)
		}
		if completed == nil {
			select {
			case <-fixture.ctx.Done():
				t.Fatal(fixture.ctx.Err())
			case <-time.After(100 * time.Millisecond):
			}
		}
	}
	if completed.SessionID != started.SessionId {
		t.Fatalf("Run Session = %q, want %q", completed.SessionID, started.SessionId)
	}
	assertDeepSeekRun(t, completed)
	if answer := fixture.finalAnswer(t, started.RunId); strings.TrimSpace(answer) != liveScheduleMarker {
		t.Fatalf("manual Schedule answer = %q, want %q", answer, liveScheduleMarker)
	}
	if _, err := fixture.runtime.GetSession(
		fixture.ctx, protocol.GetSessionRequest{SessionID: started.SessionId}, protocol.CallOptions{},
	); err != nil {
		t.Fatal(err)
	}

	providers, err := fixture.runtime.ListPluginProviders(fixture.ctx, protocol.CallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	installations, err := fixture.runtime.ListPlugins(fixture.ctx, protocol.CallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var target protocol.PluginInstallation
	var contribution string
	for _, provider := range providers.Data {
		if provider.Role != "schedules" {
			continue
		}
		contribution = provider.Contribution
		for _, installation := range installations.Data {
			if installation.ID == provider.InstallationID {
				target = installation
			}
		}
	}
	contracts, err := plugincontract.ParseCapability([]byte(testsupport.SchedulesCapabilityJSON))
	if err != nil {
		t.Fatal(err)
	}
	input, err := json.Marshal(testsupport.SchedulesListInput{})
	if err != nil {
		t.Fatal(err)
	}
	reply, err := fixture.runtime.CallPlugin(fixture.ctx, protocol.CallPluginRequest{
		Route: protocol.PluginCallRoute{
			InstallationID: target.ID,
			Digest:         target.Selected.Digest,
			Contribution:   contribution,
			Contract:       string(contracts.Identity().Bytes()),
			Method:         testsupport.SchedulesListV2Method,
			Target:         framecall.Target{Scope: extension.ScopeInstallation, Reference: target.ID},
		},
		Input: string(input),
	}, protocol.PluginCallOptions{})
	if err != nil || reply.Failure != nil {
		t.Fatalf("installed schedules: %+v, %v", reply, err)
	}
	var schedules testsupport.SchedulesPage
	if err := json.Unmarshal([]byte(reply.Output), &schedules); err != nil {
		t.Fatal(err)
	}
	if len(schedules.Rows) != 1 {
		t.Fatalf("Schedules: %+v", schedules)
	}
	current := schedules.Rows[0]
	if current.Id != created.Id || liveScheduleRevision(t, current.Revision) != liveScheduleRevision(t, created.Revision)+1 || current.LastRunAt == nil {
		t.Fatalf("Schedule after manual Run: %+v", current)
	}
	if current.NextRunAt == nil || !liveScheduleTime(t, string(*current.NextRunAt)).Equal(liveScheduleTime(t, string(*created.NextRunAt))) {
		t.Fatalf("Schedule next cursor: %v, want %v", current.NextRunAt, *created.NextRunAt)
	}
}

func TestLiveDeepSeekGoalAndPlan(t *testing.T) {
	fixture := newLiveDeepSeekFixture(t, 3*time.Minute)
	session := fixture.createSession(t, "Live DeepSeek Goal and Plan E2E")

	goal, err := fixture.runtime.StartGoal(fixture.ctx, protocol.StartGoalRequest{
		SessionID: session.ID,
		Objective: "Use set_plan exactly once to create one completed step whose description is " +
			liveGoalMarker + ". Then call report_goal_outcome with outcome completed. " +
			"Do not ask the user, run shell commands, edit files, or perform any other work.",
	}, protocol.CommandOptions{})
	if err != nil {
		t.Fatal(err)
	}
	lastGoal := goal
	for {
		current, getErr := fixture.runtime.GetGoal(
			fixture.ctx, protocol.GetGoalRequest{SessionID: session.ID}, protocol.CallOptions{},
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
		fixture.ctx, protocol.GetPlanRequest{SessionID: session.ID}, protocol.CallOptions{},
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
		fixture.ctx, protocol.ListRunsRequest{SessionID: session.ID}, protocol.CallOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs.Data) != 1 || runs.Data[0].Status != protocol.RunStatusFinished ||
		runs.Data[0].Outcome == nil || runs.Data[0].Outcome.Type != protocol.OutcomeCompleted {
		t.Fatalf("Goal Runs = %+v, want one completed Run", runs.Data)
	}
	assertDeepSeekRun(t, &runs.Data[0])
	if lastGoal.Used.CostUSD == nil {
		t.Fatal("Goal completed without exact DeepSeek pricing")
	}
	t.Logf("Goal settled after %d steps for $%.6f; final observable state was %s", lastGoal.Used.Steps, *lastGoal.Used.CostUSD, lastGoal.Status)
}

func TestLiveDeepSeekSteerAtToolBoundary(t *testing.T) {
	fixture := newLiveDeepSeekFixture(t, 3*time.Minute)
	if _, err := fixture.runtime.SetApprovalMode(
		fixture.ctx, protocol.SetApprovalModeRequest{Mode: protocol.ApprovalModeYolo}, protocol.CommandOptions{},
	); err != nil {
		t.Fatal(err)
	}
	session := fixture.createSession(t, "Live DeepSeek Steer E2E")

	started, events, err := fixture.runtime.StartRun(fixture.ctx, protocol.StartRunRequest{
		SessionID: session.ID,
		Input: []protocol.ContentBlock{{
			Type: protocol.ContentBlockText,
			Text: "Call shell exactly once with command `sleep 3` and a concise description. " +
				"After it finishes, answer exactly ORIGINAL_RESPONSE. Do not use any other tool. " +
				"Obey a later user steer instruction over this prompt.",
		}},
	}, protocol.RunCommandOptions{IdempotencyKey: rand.Text()})
	if err != nil {
		t.Fatal(err)
	}
	shellStarted, steerSent := false, false
	for event, eventErr := range events {
		if eventErr != nil {
			t.Fatal(eventErr)
		}
		if event.Event.Type != runevent.ItemStarted || event.Event.Item == nil ||
			event.Event.Item.Type != protocol.ItemTypeToolCall || event.Event.Item.Tool == nil ||
			event.Event.Item.Tool.Name != "shell" {
			continue
		}
		shellStarted = true
		if steerSent {
			continue
		}
		if _, err := fixture.runtime.SteerRun(fixture.ctx, protocol.SteerRunRequest{
			RunID:             started.RunID,
			ExpectedSegmentID: started.SegmentID,
			Input: []protocol.ContentBlock{{
				Type: protocol.ContentBlockText,
				Text: "Override the earlier final-answer instruction. After the running shell settles, " +
					"reply with exactly " + liveSteerMarker + " and nothing else.",
			}},
		}, protocol.CommandOptions{IdempotencyKey: rand.Text()}); err != nil {
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

	started, events, err := fixture.runtime.StartRun(fixture.ctx, protocol.StartRunRequest{
		SessionID: session.ID,
		Input: []protocol.ContentBlock{{
			Type: protocol.ContentBlockText,
			Text: "Call ask_user exactly once with one free-text question asking for a release codename. " +
				"After the tool returns, reply with exactly " + liveHITLMarker + " and nothing else. " +
				"Do not use another tool and do not answer before asking the question.",
		}},
	}, protocol.RunCommandOptions{RequestMeta: requestMeta, IdempotencyKey: rand.Text()})
	if err != nil {
		t.Fatal(err)
	}
	for _, eventErr := range events {
		if eventErr != nil {
			t.Fatal(eventErr)
		}
	}
	waiting, err := fixture.runtime.GetRun(
		fixture.ctx, protocol.GetRunRequest{RunID: started.RunID}, protocol.CallOptions{RequestMeta: requestMeta},
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
	}, protocol.RunCommandOptions{RequestMeta: requestMeta, IdempotencyKey: rand.Text()})
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
	dataDirectory := filepath.Join(testsupport.TempDirectory(t), "runtime")
	userHome := testsupport.TempDirectory(t)
	coordinationDirectory := testsupport.TempDirectory(t)
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

	recovered, err := runtime.GetRun(ctx, protocol.GetRunRequest{RunID: state.RunID}, protocol.CallOptions{})
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
	}, protocol.CommandOptions{}); err != nil {
		t.Fatal(err)
	}
	fixture := liveDeepSeekFixture{
		ctx:       ctx,
		runtime:   runtime,
		workspace: requiredLiveCrashPath(t, liveCrashWorkspaceEnvironment),
	}
	session := fixture.createSession(t, "Live DeepSeek crash recovery helper")
	toolMarkerPath := requiredLiveCrashPath(t, liveCrashToolMarkerEnvironment)

	started, events, err := runtime.StartRun(ctx, protocol.StartRunRequest{
		SessionID: session.ID,
		Input: []protocol.ContentBlock{{
			Type: protocol.ContentBlockText,
			Text: "Call shell exactly once with this exact command: `touch " + toolMarkerPath + " && sleep 8`. " +
				"After it finishes, reply exactly UNREACHABLE_HELPER_RESPONSE. Do not use another tool.",
		}},
	}, protocol.RunCommandOptions{IdempotencyKey: rand.Text()})
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
		"Keep the exact origin marker "+liveOriginMarker+" in this current conversation context for a later turn. "+
			"This is not a request for persistent or cross-session memory; do not write a file or call a tool. "+
			"Reply with exactly ACK_01 and nothing else.")
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
	}, protocol.CallOptions{})
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
		fixture.ctx, protocol.SessionUsageRequest{SessionID: session.ID}, protocol.CallOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if usage.Total.CostUSD != nil {
		t.Logf("13 real turns compacted once: known input=%d total cost=$%.6f", usage.Total.InputTokens, *usage.Total.CostUSD)
	} else {
		t.Logf("13 real turns compacted once: known input=%d total cost unavailable; usage recorded %d/%d, priced %d/%d", usage.Total.InputTokens, usage.Coverage.ReportedCalls, usage.Coverage.Calls, usage.Coverage.PricedCalls, usage.Coverage.Calls)
	}
}
