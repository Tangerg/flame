package runtimefixture

import (
	"context"
	"errors"
	"math"
	"slices"
	"testing"
	"time"

	"github.com/Tangerg/flame/cli/internal/agent"
	"github.com/Tangerg/flame/cli/internal/exactint"
)

func TestMockIdentitySequenceDoesNotWrapAndOverwriteExistingSession(t *testing.T) {
	runtime := New()
	runtime.identities.value.SetUint64(math.MaxUint64)
	runtime.sessions["ses_mock_0"] = &sessionState{meta: agent.Session{ID: "ses_mock_0", Title: "existing"}}

	created, err := runtime.CreateSession(t.Context(), agent.CreateSession{Workspace: "/tmp/mock"})
	if err != nil {
		t.Fatal(err)
	}
	if created.ID != "ses_mock_18446744073709551616" || runtime.sessions["ses_mock_0"].meta.Title != "existing" {
		t.Fatalf("created session %q overwrote the existing wrapped identity", created.ID)
	}
}

func TestMockSessionUpdateRevisionExhaustionIsAtomic(t *testing.T) {
	runtime := New()
	state := runtime.sessions["ses_demo_1"]
	state.meta.Revision = exactint.Maximum
	original := state.meta
	replacement := "replacement title"

	if _, err := runtime.UpdateSession(t.Context(), agent.UpdateSession{
		SessionID: state.meta.ID, ExpectedRevision: exactint.Maximum, Title: &replacement,
	}); err == nil {
		t.Fatal("session update accepted exhausted revision")
	}
	if !state.meta.Equal(original) {
		t.Fatalf("session after exhausted update = %+v, want %+v", state.meta, original)
	}
}

func TestMockSessionRollbackRevisionExhaustionIsAtomic(t *testing.T) {
	runtime := New()
	state := runtime.sessions["ses_demo_1"]
	state.meta.Revision = exactint.Maximum
	originalMeta := state.meta
	originalRuns := slices.Clone(state.runs)
	originalItems := len(state.items)
	originalRuntimeRuns := len(runtime.runs)

	if _, err := runtime.RollbackSession(t.Context(), agent.RollbackSession{
		SessionID: state.meta.ID, Scope: agent.RestoreHistory,
	}); err == nil {
		t.Fatal("session rollback accepted exhausted revision")
	}
	if !state.meta.Equal(originalMeta) || !slices.Equal(state.runs, originalRuns) ||
		len(state.items) != originalItems || len(runtime.runs) != originalRuntimeRuns {
		t.Fatalf("rollback exhaustion partially mutated session: meta %+v runs %v items %d runtime runs %d",
			state.meta, state.runs, len(state.items), len(runtime.runs))
	}
}

func TestMockSteerRevisionExhaustionDoesNotEmitPartialEvent(t *testing.T) {
	runtime := New()
	session := runtime.sessions["ses_demo_1"]
	session.meta.Revision = exactint.Maximum
	originalItems := len(session.items)
	segment := &segmentState{id: "seg_exhausted", changed: make(chan struct{})}
	run := &runState{
		id: "run_exhausted", sessionID: session.meta.ID, lineage: agent.RootRunLineage(),
		status: agent.RunStatusRunning, active: segment.id,
		segments: map[string]*segmentState{segment.id: segment}, cancel: make(chan struct{}),
	}
	runtime.runs[run.id] = run
	session.active = run.id

	err := runtime.SteerRun(t.Context(), agent.SteerRun{
		RunID: run.id, SegmentID: segment.id, Message: agent.Message{Text: "do not partially emit"},
	})
	if !errors.Is(err, errSessionRevisionExhausted) {
		t.Fatalf("steer after revision exhaustion error = %v", err)
	}
	if session.meta.Revision != exactint.Maximum || len(session.items) != originalItems || len(segment.events) != 0 {
		t.Fatalf("exhausted steer mutated revision/items/events = %d/%d/%d",
			session.meta.Revision, len(session.items), len(segment.events))
	}
}

func TestMockStartRunRevisionExhaustionIsAtomic(t *testing.T) {
	runtime := New()
	session, err := runtime.CreateSession(t.Context(), agent.CreateSession{Workspace: "/tmp/mock"})
	if err != nil {
		t.Fatal(err)
	}
	state := runtime.sessions[session.ID]
	state.meta.Revision = exactint.Maximum
	originalRuntimeRuns := len(runtime.runs)

	_, err = runtime.StartRun(t.Context(), unlimitedStartRun(session.ID, "do not partially start"))
	if !errors.Is(err, errSessionRevisionExhausted) {
		t.Fatalf("start after revision exhaustion error = %v", err)
	}
	if state.meta.Status != agent.SessionIdle || state.meta.Revision != exactint.Maximum ||
		state.active != "" || len(state.runs) != 0 || len(state.items) != 0 || len(runtime.runs) != originalRuntimeRuns {
		t.Fatalf("start exhaustion partially mutated session: meta %+v active %q runs %v items %d runtime runs %d",
			state.meta, state.active, state.runs, len(state.items), len(runtime.runs))
	}
}

func TestMockBackgroundEventRevisionExhaustionTerminatesTheSegmentWithoutPartialEvent(t *testing.T) {
	runtime := New()
	runtime.Script = func(string) Script {
		return Script{Prelude: []Step{
			eventStep(0, agent.BlockCompleted{Block: agent.Block{ID: "answer", Kind: agent.BlockAssistant, Text: "must not commit"}}),
			eventStep(0, agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}),
		}}
	}
	session, err := runtime.CreateSession(t.Context(), agent.CreateSession{Workspace: "/tmp/mock"})
	if err != nil {
		t.Fatal(err)
	}
	state := runtime.sessions[session.ID]
	state.meta.Revision = exactint.Maximum - uint64(startRunRevisionChanges(state))

	opened, err := runtime.StartRun(t.Context(), unlimitedStartRun(session.ID, "exhaust after opening"))
	if err != nil {
		t.Fatal(err)
	}
	events, streamErr := collectSegment(opened)
	if !errors.Is(streamErr, errSessionRevisionExhausted) {
		t.Fatalf("background event exhaustion stream error = %v", streamErr)
	}
	if state.meta.Revision != exactint.Maximum || len(events) != 2 || len(state.items) != 1 {
		t.Fatalf("background exhaustion committed partial event: revision %d events %d items %d",
			state.meta.Revision, len(events), len(state.items))
	}
	if run := runtime.runs[opened.RunID]; run.status != agent.RunStatusRunning || run.active != opened.SegmentID {
		t.Fatalf("background exhaustion invented lifecycle transition: %+v", projectRun(run))
	}
}

func TestMockParkRevisionExhaustionDoesNotPublishAPartialWaitingSet(t *testing.T) {
	runtime := New()
	runtime.Instant = true
	runtime.Script = func(string) Script {
		return Script{
			Interactions: []agent.Interaction{approvalFixture("approval", "approve")},
			Continue: func([]agent.InterruptAnswer) []Step {
				return []Step{eventStep(0, agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}})}
			},
		}
	}
	session, err := runtime.CreateSession(t.Context(), agent.CreateSession{Workspace: "/tmp/mock"})
	if err != nil {
		t.Fatal(err)
	}
	state := runtime.sessions[session.ID]
	state.meta.Revision = exactint.Maximum - uint64(startRunRevisionChanges(state))

	opened, err := runtime.StartRun(t.Context(), unlimitedStartRun(session.ID, "cannot partially wait"))
	if err != nil {
		t.Fatal(err)
	}
	_, streamErr := collectSegment(opened)
	if !errors.Is(streamErr, errSessionRevisionExhausted) {
		t.Fatalf("park exhaustion stream error = %v", streamErr)
	}
	run := runtime.runs[opened.RunID]
	if state.meta.Status != agent.SessionRunning || run.status != agent.RunStatusRunning ||
		len(run.interactions) != 0 || len(run.answers) != 0 || len(state.items) != 1 {
		t.Fatalf("park exhaustion published partial waiting state: meta %+v run %+v interactions %d answers %d items %d",
			state.meta, projectRun(run), len(run.interactions), len(run.answers), len(state.items))
	}
}

func TestMockFinishRevisionExhaustionLeavesTheRunExecutingAndReportsTheStreamFailure(t *testing.T) {
	runtime := New()
	runtime.Script = func(string) Script {
		return Script{Prelude: []Step{eventStep(0, agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}})}}
	}
	session, err := runtime.CreateSession(t.Context(), agent.CreateSession{Workspace: "/tmp/mock"})
	if err != nil {
		t.Fatal(err)
	}
	state := runtime.sessions[session.ID]
	state.meta.Revision = exactint.Maximum - uint64(startRunRevisionChanges(state)) - uint64(sessionEventRevisionChange())

	opened, err := runtime.StartRun(t.Context(), unlimitedStartRun(session.ID, "cannot partially finish"))
	if err != nil {
		t.Fatal(err)
	}
	_, streamErr := collectSegment(opened)
	if !errors.Is(streamErr, errSessionRevisionExhausted) {
		t.Fatalf("finish exhaustion stream error = %v", streamErr)
	}
	run := runtime.runs[opened.RunID]
	if state.meta.Status != agent.SessionRunning || run.status != agent.RunStatusRunning ||
		run.active != opened.SegmentID || run.outcome.Status != "" {
		t.Fatalf("finish exhaustion partially transitioned session/run: %+v / %+v", state.meta, projectRun(run))
	}
}

func TestMockCancelRevisionExhaustionIsAtomic(t *testing.T) {
	runtime := New()
	runtime.Script = func(string) Script {
		return Script{Prelude: []Step{eventStep(time.Hour, agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}})}}
	}
	session, err := runtime.CreateSession(t.Context(), agent.CreateSession{Workspace: "/tmp/mock"})
	if err != nil {
		t.Fatal(err)
	}
	state := runtime.sessions[session.ID]
	state.meta.Revision = exactint.Maximum - uint64(startRunRevisionChanges(state)) - uint64(sessionEventRevisionChange())
	opened, err := runtime.StartRun(t.Context(), unlimitedStartRun(session.ID, "cannot partially cancel"))
	if err != nil {
		t.Fatal(err)
	}
	originalMeta := state.meta
	originalRun := projectRun(runtime.runs[opened.RunID])
	originalEvents := len(runtime.runs[opened.RunID].segments[opened.SegmentID].events)

	_, err = runtime.CancelRun(t.Context(), agent.CancelRun{RunID: opened.RunID})
	if !errors.Is(err, errSessionRevisionExhausted) {
		t.Fatalf("cancel after revision exhaustion error = %v", err)
	}
	run := runtime.runs[opened.RunID]
	if !state.meta.Equal(originalMeta) || !projectRun(run).Equal(originalRun) ||
		len(run.segments[opened.SegmentID].events) != originalEvents {
		t.Fatalf("cancel exhaustion partially mutated session/run: %+v / %+v", state.meta, projectRun(run))
	}
	select {
	case <-run.cancel:
		t.Fatal("cancel exhaustion closed the run cancellation signal")
	default:
	}
}

func collectSegment(stream agent.SegmentStream) ([]agent.RunEvent, error) {
	var events []agent.RunEvent
	for event, err := range stream.Events {
		if err != nil {
			return events, err
		}
		events = append(events, event)
	}
	return events, nil
}

func TestRuntimeStartResumeAndColdRestore(t *testing.T) {
	runtime := New()
	runtime.Instant = true
	session, err := runtime.CreateSession(t.Context(), agent.CreateSession{Workspace: "/tmp/mock"})
	if err != nil {
		t.Fatal(err)
	}
	opened, conversation := startWaitingRun(t, runtime, session.ID)
	interaction := requireWaitingProjection(t, runtime, session.ID, opened)
	resumeApprovedRun(t, runtime, opened, conversation, interaction)
	requireCompletedColdProjection(t, runtime, session.ID, opened.RunID, interaction)
}

func TestMockResumeRevisionExhaustionIsAtomic(t *testing.T) {
	runtime := New()
	runtime.Instant = true
	runtime.Script = func(string) Script {
		return Script{
			Interactions: []agent.Interaction{approvalFixture("approval", "approve")},
			Continue: func([]agent.InterruptAnswer) []Step {
				return []Step{eventStep(0, agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}})}
			},
		}
	}
	session, err := runtime.CreateSession(t.Context(), agent.CreateSession{Workspace: "/tmp/mock"})
	if err != nil {
		t.Fatal(err)
	}
	opened, err := runtime.StartRun(t.Context(), unlimitedStartRun(session.ID, "wait for approval"))
	if err != nil {
		t.Fatal(err)
	}
	conversation := agent.NewConversation()
	drain(t, opened, conversation)
	interaction := conversation.Interactions()[0]
	state := runtime.sessions[session.ID]
	state.meta.Revision = exactint.Maximum
	run := runtime.runs[opened.RunID]
	originalMeta := state.meta
	originalRun := projectRun(run)
	originalItems := len(state.items)
	originalAnswers := len(run.answers)
	originalSegments := len(run.segments)
	originalRules := len(runtime.rules)

	_, err = runtime.ResumeRun(t.Context(), agent.ResumeRun{RunID: opened.RunID, Answers: []agent.InterruptAnswer{{
		ItemID: agent.InteractionItemID(interaction), Answer: agent.ApprovalAnswer{Decision: agent.ApprovalApprove},
	}}})
	if !errors.Is(err, errSessionRevisionExhausted) {
		t.Fatalf("resume after revision exhaustion error = %v", err)
	}
	if !state.meta.Equal(originalMeta) || !projectRun(run).Equal(originalRun) || len(state.items) != originalItems ||
		len(run.answers) != originalAnswers || len(run.segments) != originalSegments || len(runtime.rules) != originalRules {
		t.Fatalf("resume exhaustion partially mutated state: meta %+v run %+v items %d answers %d segments %d rules %d",
			state.meta, projectRun(run), len(state.items), len(run.answers), len(run.segments), len(runtime.rules))
	}
}

func unlimitedStartRun(sessionID, text string) agent.StartRun {
	return agent.StartRun{
		SessionID: sessionID, Message: agent.Message{Text: text},
		Options: agent.RunOptions{Limits: agent.UnlimitedRunLimits()},
	}
}

func TestApprovalCatalogRejectsEmptyIdentitiesConsistently(t *testing.T) {
	t.Parallel()

	runtime := New()
	if _, err := runtime.ListApprovalRules(t.Context(), "  "); err == nil {
		t.Fatal("empty session identity was accepted")
	}
	if err := runtime.DeleteApprovalRule(t.Context(), "\t"); err == nil {
		t.Fatal("empty rule identity was accepted")
	}
}

func TestSessionCatalogRejectsInvalidLocalFilters(t *testing.T) {
	t.Parallel()

	runtime := New()
	for _, query := range []agent.SessionQuery{
		{PageSize: agent.DefaultPageSize(), Workspace: "relative/workspace"},
	} {
		if _, err := runtime.ListSessions(t.Context(), query); err == nil {
			t.Fatalf("ListSessions accepted %+v", query)
		}
	}
}

func TestProjectApprovalRulesFollowTheResolvedProjectRoot(t *testing.T) {
	t.Parallel()

	runtime := New()
	runtime.mu.Lock()
	runtime.sessions["ses_demo_1"].meta.Workspace.ProjectRoot = "/tmp/demo"
	runtime.sessions["ses_demo_2"].meta.Workspace.ProjectRoot = "/tmp/demo"
	runtime.rules = []storedRule{{view: agent.ApprovalRule{
		ID: "rule_project", Scope: agent.RememberProject, Dir: "/tmp/demo",
		Tool: "shell", Subject: "go test ./...", Decision: agent.ApprovalRuleAllow,
	}}}
	runtime.mu.Unlock()

	for _, sessionID := range []string{"ses_demo_1", "ses_demo_2"} {
		rules, err := runtime.ListApprovalRules(t.Context(), sessionID)
		if err != nil || len(rules) != 1 {
			t.Fatalf("project rules for %s = %+v, %v", sessionID, rules, err)
		}
	}
	rules, err := runtime.ListApprovalRules(t.Context(), "ses_demo_3")
	if err != nil || len(rules) != 0 {
		t.Fatalf("unrelated project rules = %+v, %v", rules, err)
	}
}

func startWaitingRun(t *testing.T, runtime *Runtime, sessionID string) (agent.SegmentStream, *agent.Conversation) {
	t.Helper()
	maxSteps, maxBudget := 12, 1.5
	limits, err := agent.NewRunLimits(agent.RunLimitValues{MaxSteps: &maxSteps, MaxBudgetUSD: &maxBudget})
	if err != nil {
		t.Fatal(err)
	}
	opened, err := runtime.StartRun(t.Context(), agent.StartRun{
		SessionID: sessionID, Message: agent.Message{Text: "fix the flaky test"},
		Options: agent.RunOptions{Provider: "mock", Model: "balanced", Limits: limits},
	})
	if err != nil {
		t.Fatal(err)
	}
	conversation := agent.NewConversation()
	drain(t, opened, conversation)
	if conversation.Phase() != agent.ConversationWaiting || len(conversation.Interactions()) != 1 {
		t.Fatalf("after first segment: phase %v, interactions %d", conversation.Phase(), len(conversation.Interactions()))
	}
	return opened, conversation
}

func requireWaitingProjection(t *testing.T, runtime *Runtime, sessionID string, opened agent.SegmentStream) agent.Interaction {
	t.Helper()
	waiting, err := runtime.GetSession(t.Context(), sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(waiting.Interactions) != 1 {
		t.Fatalf("waiting interactions = %d, want 1", len(waiting.Interactions))
	}
	waitingRun, ok := waiting.LatestRun()
	if !ok {
		t.Fatal("waiting snapshot has no run")
	}
	interaction := waiting.Interactions[0]
	approvalItem, ok := snapshotBlock(waiting, opened.RunID, agent.InteractionItemID(interaction))
	if !ok || approvalItem.Kind != agent.BlockTool || approvalItem.Status != agent.BlockStatusRunning || waitingRun.Usage.InputTokens == 0 {
		t.Fatalf("waiting approval projection = item %+v, usage %+v", approvalItem, waitingRun.Usage)
	}
	return interaction
}

func resumeApprovedRun(t *testing.T, runtime *Runtime, opened agent.SegmentStream, conversation *agent.Conversation, interaction agent.Interaction) {
	t.Helper()
	continued, err := runtime.ResumeRun(t.Context(), agent.ResumeRun{
		RunID: opened.RunID,
		Answers: []agent.InterruptAnswer{{
			ItemID: agent.InteractionItemID(interaction),
			Answer: agent.ApprovalAnswer{Decision: agent.ApprovalApprove, Remember: agent.RememberProject},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if continued.SegmentID == opened.SegmentID {
		t.Fatal("resume reused the first segment")
	}
	drain(t, continued, conversation)
	if conversation.Outcome().Status != agent.OutcomeCompleted {
		t.Fatalf("outcome = %+v", conversation.Outcome())
	}
}

func requireCompletedColdProjection(t *testing.T, runtime *Runtime, sessionID, runID string, interaction agent.Interaction) {
	t.Helper()
	snapshot, err := runtime.GetSession(t.Context(), sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if _, active := snapshot.ActiveRun(); active || len(snapshot.Interactions) != 0 || len(snapshot.Transcript) < 4 {
		t.Fatalf("snapshot = runs %+v, interactions %d, transcript %d", snapshot.Runs, len(snapshot.Interactions), len(snapshot.Transcript))
	}
	latest, ok := snapshot.LatestRun()
	if !ok {
		t.Fatal("latest run is missing")
	}
	steps, stepsLimited := latest.Limits.MaxSteps()
	budget, budgetLimited := latest.Limits.MaxBudgetUSD()
	if !stepsLimited || steps != 12 || !budgetLimited || budget != 1.5 {
		t.Fatalf("latest run limits = %+v", latest.Limits)
	}
	approvalItem, ok := snapshotBlock(snapshot, runID, agent.InteractionItemID(interaction))
	if !ok || approvalItem.Status != agent.BlockStatusCompleted || approvalItem.Tool.Status != agent.ToolOK {
		t.Fatalf("completed approval item = %+v", approvalItem)
	}
	rules, err := runtime.ListApprovalRules(t.Context(), sessionID)
	if err != nil || len(rules) != 1 || rules[0].Scope != agent.RememberProject {
		t.Fatalf("rules = %+v, %v", rules, err)
	}
}

func TestRuntimeReconnectUsesOpaqueReplayCheckpoint(t *testing.T) {
	runtime := New()
	runtime.Faults = []SubscriptionFault{{Kind: FaultDisconnect, After: 1}}
	runtime.Script = func(string) Script {
		return Script{Prelude: []Step{
			eventStep(30*time.Millisecond, agent.BlockCompleted{Block: agent.Block{ID: "answer", Kind: agent.BlockAssistant, Text: "done"}}),
			eventStep(0, agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}),
		}}
	}
	session, _ := runtime.CreateSession(t.Context(), agent.CreateSession{Workspace: "/tmp/mock"})
	opened, err := runtime.StartRun(t.Context(), unlimitedStartRun(session.ID, "hello"))
	if err != nil {
		t.Fatal(err)
	}
	conversation := agent.NewConversation()
	var disconnected error
	for event, streamErr := range opened.Events {
		if streamErr != nil {
			disconnected = streamErr
			break
		}
		if _, applyRunEventErr := conversation.ApplyRunEvent(event); applyRunEventErr != nil {
			t.Fatal(applyRunEventErr)
		}
	}
	if !errors.Is(disconnected, agent.ErrDisconnected) {
		t.Fatalf("stream error = %v", disconnected)
	}
	checkpoint := conversation.Checkpoint()
	if checkpoint == "" {
		t.Fatal("no replay checkpoint was retained")
	}
	rebound, err := runtime.SubscribeRun(t.Context(), agent.SubscribeRun{
		RunID: opened.RunID, SegmentID: opened.SegmentID, AfterEventID: checkpoint,
	})
	if err != nil {
		t.Fatal(err)
	}
	drain(t, rebound, conversation)
	if conversation.Outcome().Status != agent.OutcomeCompleted {
		t.Fatalf("outcome = %+v", conversation.Outcome())
	}
}

func TestRuntimeSubscribeWithoutCheckpointAttachesAtHead(t *testing.T) {
	runtime := New()
	runtime.Script = func(string) Script {
		return Script{Prelude: []Step{eventStep(time.Second, agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}})}}
	}
	session, _ := runtime.CreateSession(t.Context(), agent.CreateSession{Workspace: "/tmp/mock"})
	opened, err := runtime.StartRun(t.Context(), unlimitedStartRun(session.ID, "hello"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := timeLimitedContext(t, 20*time.Millisecond)
	defer cancel()
	attached, err := runtime.SubscribeRun(ctx, agent.SubscribeRun{RunID: opened.RunID, SegmentID: opened.SegmentID})
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for event, streamErr := range attached.Events {
		if streamErr != nil {
			break
		}
		count++
		_ = event
	}
	if count != 0 {
		t.Fatalf("attach-at-head replayed %d historical events", count)
	}
	_, _ = runtime.CancelRun(t.Context(), agent.CancelRun{RunID: opened.RunID})
}

func TestRuntimeForkStartsWithAFreshProjectionAtRunBoundary(t *testing.T) {
	runtime := New()
	forked, err := runtime.ForkSession(t.Context(), agent.ForkSession{SessionID: "ses_demo_1", FromRunID: "run_demo_history"})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := runtime.GetSession(t.Context(), forked.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Transcript) != 0 || len(snapshot.Runs) != 0 {
		t.Fatalf("fork projection = %d blocks, %d runs", len(snapshot.Transcript), len(snapshot.Runs))
	}
}

func TestRuntimeRollbackRestoresTheEarliestDroppedOpeningInput(t *testing.T) {
	runtime := New()
	result, err := runtime.RollbackSession(t.Context(), agent.RollbackSession{
		SessionID: "ses_demo_1", Scope: agent.RestoreHistory,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Dropped) != 1 {
		t.Fatalf("dropped runs = %+v", result.Dropped)
	}
	input, ok := result.FirstOpeningInput()
	text, images := input.OpeningText()
	if !ok || text != "Why is the cache expiry test flaky?" || images != 0 {
		t.Fatalf("opening input = (%q, %d, %t)", text, images, ok)
	}
	snapshot, err := runtime.GetSession(t.Context(), "ses_demo_1")
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Runs) != 0 || len(snapshot.Transcript) != 0 {
		t.Fatalf("snapshot after rollback = %+v", snapshot)
	}
}

func TestRuntimeForkExcludesAnActiveTail(t *testing.T) {
	runtime := New()
	runtime.Script = func(string) Script {
		return Script{Prelude: []Step{eventStep(time.Hour, agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}})}}
	}
	opened, err := runtime.StartRun(t.Context(), unlimitedStartRun("ses_demo_1", "active tail"))
	if err != nil {
		t.Fatal(err)
	}
	forked, err := runtime.ForkSession(t.Context(), agent.ForkSession{SessionID: "ses_demo_1"})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := runtime.GetSession(t.Context(), forked.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Transcript) != 0 || len(snapshot.Runs) != 0 {
		t.Fatalf("fork copied a parent projection: blocks=%+v runs=%+v", snapshot.Transcript, snapshot.Runs)
	}
	if _, err := runtime.ForkSession(t.Context(), agent.ForkSession{SessionID: "ses_demo_1", FromRunID: opened.RunID}); !errors.Is(err, agent.ErrRunNotFound) {
		t.Fatalf("explicit active boundary error = %v", err)
	}
	_, _ = runtime.CancelRun(t.Context(), agent.CancelRun{RunID: opened.RunID})
}

func TestRuntimeForkCopiesThePlanAtItsRunBoundary(t *testing.T) {
	runtime := New()
	runtime.Script = func(prompt string) Script {
		plan := []agent.PlanItem{{Title: prompt + " plan", Status: agent.PlanActive}}
		delay := time.Duration(0)
		if prompt == "active" {
			delay = time.Hour
		}
		return Script{Prelude: []Step{
			replacePlanStep(0, plan),
			eventStep(delay, agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}),
		}}
	}
	session, err := runtime.CreateSession(t.Context(), agent.CreateSession{Workspace: "/tmp/mock"})
	if err != nil {
		t.Fatal(err)
	}
	first, err := runtime.StartRun(t.Context(), unlimitedStartRun(session.ID, "boundary"))
	if err != nil {
		t.Fatal(err)
	}
	drain(t, first, agent.NewConversation())
	second, err := runtime.StartRun(t.Context(), unlimitedStartRun(session.ID, "active"))
	if err != nil {
		t.Fatal(err)
	}
	for event, streamErr := range second.Events {
		if streamErr != nil {
			t.Fatal(streamErr)
		}
		if _, changed := event.Event.(agent.PlanChanged); changed {
			break
		}
	}
	forked, err := runtime.ForkSession(t.Context(), agent.ForkSession{SessionID: session.ID})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := runtime.GetSession(t.Context(), forked.ID)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Plan == nil || snapshot.Plan.Revision() != 1 || len(snapshot.Plan.Items()) != 1 || snapshot.Plan.Items()[0].Title != "boundary plan" {
		t.Fatalf("fork plan = %+v", snapshot.Plan)
	}
	if len(snapshot.Transcript) != 0 || len(snapshot.Runs) != 0 {
		t.Fatalf("fork copied a parent projection: blocks=%+v runs=%+v", snapshot.Transcript, snapshot.Runs)
	}
	_, _ = runtime.CancelRun(t.Context(), agent.CancelRun{RunID: second.RunID})
}

func TestRuntimeColdReadTracksAndSettlesRunningItems(t *testing.T) {
	runtime := New()
	runtime.Script = func(string) Script {
		return Script{Prelude: []Step{
			eventStep(0, agent.BlockStarted{Block: agent.Block{ID: "answer", Kind: agent.BlockAssistant}}),
			eventStep(0, agent.BlockStarted{Block: agent.Block{ID: "tool", Kind: agent.BlockTool, Tool: &agent.ToolCall{Kind: agent.ToolShell, Name: "shell", Status: agent.ToolRunning}}}),
			eventStep(time.Hour, agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}),
		}}
	}
	session, err := runtime.CreateSession(t.Context(), agent.CreateSession{Workspace: "/tmp/mock"})
	if err != nil {
		t.Fatal(err)
	}
	opened, err := runtime.StartRun(t.Context(), unlimitedStartRun(session.ID, "run"))
	if err != nil {
		t.Fatal(err)
	}
	startedCount := 0
	for event, streamErr := range opened.Events {
		if streamErr != nil {
			t.Fatal(streamErr)
		}
		if _, started := event.Event.(agent.BlockStarted); started {
			startedCount++
			if startedCount == 2 {
				break
			}
		}
	}
	running, err := runtime.GetSession(t.Context(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(running.Transcript) != 2 {
		t.Fatalf("cold transcript includes a provisional assistant start: %+v", running.Transcript)
	}
	if got := running.Transcript[len(running.Transcript)-1]; got.Status != agent.BlockStatusRunning || got.ID != opened.RunID+":tool" || got.Kind != agent.BlockTool {
		t.Fatalf("running item = %+v", got)
	}
	if _, cancelRunErr := runtime.CancelRun(t.Context(), agent.CancelRun{RunID: opened.RunID}); cancelRunErr != nil {
		t.Fatal(cancelRunErr)
	}
	settled, err := runtime.GetSession(t.Context(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := settled.Transcript[len(settled.Transcript)-1]; got.Status != agent.BlockStatusIncomplete {
		t.Fatalf("settled item = %+v", got)
	}
}

func TestScriptContinuationReceivesFixtureLocalItemIDs(t *testing.T) {
	runtime := New()
	runtime.Instant = true
	var received string
	runtime.Script = func(string) Script {
		return Script{
			Interactions: []agent.Interaction{approvalFixture("approval", "approve")},
			Continue: func(answers []agent.InterruptAnswer) []Step {
				received = answers[0].ItemID
				return []Step{eventStep(0, agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}})}
			},
		}
	}
	session, _ := runtime.CreateSession(t.Context(), agent.CreateSession{Workspace: "/tmp/mock"})
	opened, err := runtime.StartRun(t.Context(), unlimitedStartRun(session.ID, "ask"))
	if err != nil {
		t.Fatal(err)
	}
	conversation := agent.NewConversation()
	drain(t, opened, conversation)
	interaction := conversation.Interactions()[0]
	continued, err := runtime.ResumeRun(t.Context(), agent.ResumeRun{RunID: opened.RunID, Answers: []agent.InterruptAnswer{{
		ItemID: agent.InteractionItemID(interaction), Answer: agent.ApprovalAnswer{Decision: agent.ApprovalApprove},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	drain(t, continued, conversation)
	if received != "approval" {
		t.Fatalf("continuation item id = %q, want fixture-local id", received)
	}
}

func TestApprovalArgumentOverrideBecomesTheCompletedToolProjection(t *testing.T) {
	runtime := New()
	runtime.Instant = true
	original := []byte(`{"command":"rm generated.txt"}`)
	runtime.Script = func(string) Script {
		return Script{
			Interactions: []agent.Interaction{agent.Approval{
				ItemID: "approval", Title: "Run command",
				Tool: &agent.ToolCall{
					Kind: agent.ToolShell, Name: "shell", Status: agent.ToolRunning, ArgumentsJSON: original,
				},
			}},
			Continue: func([]agent.InterruptAnswer) []Step {
				return []Step{eventStep(0, agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}})}
			},
		}
	}
	session, _ := runtime.CreateSession(t.Context(), agent.CreateSession{Workspace: "/tmp/mock"})
	opened, err := runtime.StartRun(t.Context(), agent.StartRun{
		SessionID: session.ID, Message: agent.Message{Text: "run safely"},
		Options: agent.RunOptions{Limits: agent.UnlimitedRunLimits()},
	})
	if err != nil {
		t.Fatal(err)
	}
	conversation := agent.NewConversation()
	drain(t, opened, conversation)
	interaction := conversation.Interactions()[0]
	override, err := agent.ParseToolArgumentOverride([]byte(`{"command":"echo safe"}`))
	if err != nil {
		t.Fatal(err)
	}
	continued, err := runtime.ResumeRun(t.Context(), agent.ResumeRun{
		RunID: opened.RunID, Answers: []agent.InterruptAnswer{{
			ItemID: agent.InteractionItemID(interaction),
			Answer: agent.ApprovalAnswer{Decision: agent.ApprovalApprove, ArgumentOverride: override},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	drain(t, continued, conversation)
	snapshot, err := runtime.GetSession(t.Context(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	completed, ok := snapshotBlock(snapshot, opened.RunID, agent.InteractionItemID(interaction))
	if !ok || completed.Tool == nil || string(completed.Tool.ArgumentsJSON) != `{"command":"echo safe"}` {
		t.Fatalf("completed edited tool = %+v", completed)
	}
	if string(original) != `{"command":"rm generated.txt"}` {
		t.Fatalf("mock mutated fixture arguments: %s", original)
	}
}

func TestInvalidFaultConfigurationDoesNotMutateRunState(t *testing.T) {
	runtime := New()
	runtime.Faults = []SubscriptionFault{{Kind: FaultKind("unknown"), After: 1}}
	session, _ := runtime.CreateSession(t.Context(), agent.CreateSession{Workspace: "/tmp/mock"})
	if _, err := runtime.StartRun(t.Context(), unlimitedStartRun(session.ID, "start")); err == nil {
		t.Fatal("invalid subscription fault was ignored")
	}
	snapshot, err := runtime.GetSession(t.Context(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Runs) != 0 || len(snapshot.Transcript) != 0 || snapshot.Session.Status != agent.SessionIdle {
		t.Fatalf("failed start mutated snapshot: %+v", snapshot)
	}
}

func TestRememberedRulesRemoveOnlyMatchedApprovalsFromThePendingSet(t *testing.T) {
	runtime := New()
	runtime.Instant = true
	runtime.rules = []storedRule{{view: agent.ApprovalRule{
		ID: "rule_1", Scope: agent.RememberGlobal, Tool: "shell", Subject: "go test ./...", Decision: agent.ApprovalRuleAllow,
	}}}
	var continuedWith []agent.InterruptAnswer
	runtime.Script = func(string) Script {
		return Script{
			Interactions: []agent.Interaction{
				func() agent.Approval {
					approval := approvalFixture("approval", "run tests")
					approval.RuleHint, approval.Rememberable = "shell:go test ./...", true
					return approval
				}(),
				agent.Question{ItemID: "question", Title: "Target", Fields: []agent.QuestionField{{Prompt: "Target", Kind: agent.QuestionText}}},
			},
			Continue: func(answers []agent.InterruptAnswer) []Step {
				continuedWith = cloneAnswers(answers)
				return []Step{eventStep(0, agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}})}
			},
		}
	}
	session, _ := runtime.CreateSession(t.Context(), agent.CreateSession{Workspace: "/tmp/mock"})
	opened, err := runtime.StartRun(t.Context(), unlimitedStartRun(session.ID, "ask"))
	if err != nil {
		t.Fatal(err)
	}
	conversation := agent.NewConversation()
	drain(t, opened, conversation)
	pending := conversation.Interactions()
	if len(pending) != 1 {
		t.Fatalf("pending interactions = %+v, want only the unmatched question", pending)
	}
	question, ok := pending[0].(agent.Question)
	if !ok {
		t.Fatalf("pending interaction = %T, want question", pending[0])
	}
	waiting, err := runtime.GetSession(t.Context(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	questionItem, found := snapshotBlock(waiting, opened.RunID, question.ItemID)
	if !found || questionItem.Kind != agent.BlockQuestion || questionItem.Question == nil {
		t.Fatalf("durable question item = %+v", questionItem)
	}
	continued, err := runtime.ResumeRun(t.Context(), agent.ResumeRun{RunID: opened.RunID, Answers: []agent.InterruptAnswer{{
		ItemID: question.ItemID, Answer: agent.QuestionAnswer{Values: [][]string{{"linux"}}},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := runtime.GetSession(t.Context(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	questionItem, found = snapshotBlock(accepted, opened.RunID, question.ItemID)
	if !found || questionItem.Question == nil || !questionItem.Question.Answered() ||
		questionItem.Question.Answers[0][0] != "linux" {
		t.Fatalf("durable accepted question = %+v", questionItem)
	}
	drain(t, continued, conversation)
	if len(continuedWith) != 2 || continuedWith[0].ItemID != "approval" || continuedWith[1].ItemID != "question" {
		t.Fatalf("continuation answers = %+v, want the complete fixture-local set", continuedWith)
	}
}

func drain(t *testing.T, stream agent.SegmentStream, conversation *agent.Conversation) {
	t.Helper()
	if err := stream.Validate(); err != nil {
		t.Fatal(err)
	}
	for event, streamErr := range stream.Events {
		if streamErr != nil {
			t.Fatal(streamErr)
		}
		if _, err := conversation.ApplyRunEvent(event); err != nil {
			t.Fatal(err)
		}
	}
}

func approvalFixture(itemID, title string) agent.Approval {
	return agent.Approval{
		ItemID: itemID, Title: title,
		Tool: &agent.ToolCall{Kind: agent.ToolShell, Name: "shell", Status: agent.ToolRunning},
	}
}

func snapshotBlock(snapshot agent.SessionSnapshot, runID, itemID string) (agent.Block, bool) {
	for _, block := range snapshot.Transcript {
		if block.RunID == runID && block.ID == itemID {
			return block, true
		}
	}
	return agent.Block{}, false
}

func timeLimitedContext(t *testing.T, timeout time.Duration) (context.Context, context.CancelFunc) {
	t.Helper()
	return context.WithTimeout(t.Context(), timeout)
}
