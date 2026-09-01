package agent

import (
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/protocol"
)

func TestConversationFailureRemainsVisibleWhenDerivedIdentityCollides(t *testing.T) {
	conversation := NewConversation()
	if err := conversation.put(Block{
		ID: "failure:2", Kind: BlockError, Status: BlockStatusIncomplete, Text: "earlier failure",
	}, true); err != nil {
		t.Fatal(err)
	}

	conversation.Failed(errors.New("latest failure"))

	blocks := conversation.Blocks()
	if len(blocks) != 2 || blocks[1].Text != "latest failure" || blocks[1].ID == blocks[0].ID {
		t.Fatalf("failure blocks = %+v, want a distinct visible latest failure", blocks)
	}
}

func TestConversationFoldsInitialAndResumedSegments(t *testing.T) {
	conversation := NewConversation()
	started := RunEvent{EventID: "opaque:start", RunID: "run_1", SegmentID: "seg_1", Event: SegmentStarted{Run: runningRun("seg_1")}}
	apply(t, conversation, started)
	apply(t, conversation, RunEvent{EventID: "opaque:item-start", RunID: "run_1", SegmentID: "seg_1", Event: BlockStarted{Block: Block{ID: "msg_1", RunID: "run_1", Status: BlockStatusRunning, Kind: BlockAssistant}}})
	apply(t, conversation, RunEvent{EventID: "opaque:delta", RunID: "run_1", SegmentID: "seg_1", Event: BlockDelta{BlockID: "msg_1", Text: "draft"}})
	if got := conversation.Checkpoint(); got != "opaque:item-start" {
		t.Fatalf("checkpoint after delta = %q", got)
	}
	apply(t, conversation, RunEvent{EventID: "opaque:item-done", RunID: "run_1", SegmentID: "seg_1", Event: BlockCompleted{Block: Block{ID: "msg_1", RunID: "run_1", Status: BlockStatusCompleted, Kind: BlockAssistant, Text: "final"}}})

	interrupts := []Interaction{
		runningApproval("item_approval", "run shell"),
		Question{RunID: "run_1", ItemID: "item_question", Title: "choose", Fields: []QuestionField{{Prompt: "Which?", Kind: QuestionSingle, Options: []QuestionOption{{Label: "A"}, {Label: "B"}}}}},
	}
	approval := interrupts[0].(Approval)
	startedApprovalTool := approval.Tool.Clone()
	startedApprovalTool.Safety = ToolSafetyExec
	startedApprovalTool.StartedAt = time.Date(2026, time.August, 31, 6, 0, 0, 0, time.UTC)
	apply(t, conversation, RunEvent{EventID: "approval-start", RunID: "run_1", SegmentID: "seg_1", Event: BlockStarted{Block: Block{
		ID: approval.ItemID, RunID: "run_1", Status: BlockStatusRunning, Kind: BlockTool, Tool: &startedApprovalTool,
	}}})
	question := interrupts[1].(Question)
	apply(t, conversation, RunEvent{EventID: "question-done", RunID: "run_1", SegmentID: "seg_1", Event: BlockCompleted{Block: Block{
		ID: question.ItemID, RunID: "run_1", Status: BlockStatusCompleted, Kind: BlockQuestion, Question: &question,
	}}})
	interruptedUsage := Usage{InputTokens: 10, OutputTokens: 2}
	interruptedContext := int64(8_192)
	apply(t, conversation, RunEvent{EventID: "opaque:park", RunID: "run_1", SegmentID: "seg_1", Event: RunInterrupted{
		Interactions: interrupts, Usage: interruptedUsage, ContextTokens: interruptedContext,
	}})
	if conversation.Phase() != ConversationWaiting || len(conversation.Interactions()) != 2 || !conversation.Usage().Equal(interruptedUsage) {
		t.Fatalf("waiting projection = phase %v, interactions %d, usage %+v", conversation.Phase(), len(conversation.Interactions()), conversation.Usage())
	}
	if runs := conversation.Runs(); len(runs) != 1 || runs[0].ContextTokens != interruptedContext {
		t.Fatalf("waiting run context = %+v, want %d", runs, interruptedContext)
	}
	if current, ok := conversation.CurrentRun(); !ok || current.ID != "run_1" || current.ContextTokens != interruptedContext {
		t.Fatalf("waiting current Run = %+v, %t", current, ok)
	}
	acceptedQuestions, err := conversation.RecordAcceptedInteractionAnswers([]InterruptAnswer{
		{ItemID: approval.ItemID, Answer: ApprovalAnswer{Decision: ApprovalApprove}},
		{ItemID: question.ItemID, Answer: QuestionAnswer{Values: [][]string{{"A"}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(acceptedQuestions) != 1 || acceptedQuestions[0].Question == nil ||
		!acceptedQuestions[0].Question.Answered() || acceptedQuestions[0].Question.Answers[0][0] != "A" {
		t.Fatalf("accepted questions = %+v", acceptedQuestions)
	}
	if conversation.Phase() != ConversationWaiting || len(conversation.Interactions()) != 2 {
		t.Fatal("accepted answers released waiting state before the continuation segment")
	}

	resumed := runningRun("seg_2")
	resumed.Usage = interruptedUsage
	resumed.ContextTokens = interruptedContext
	apply(t, conversation, RunEvent{EventID: "different-space:start", RunID: "run_1", SegmentID: "seg_2", Event: SegmentStarted{Run: resumed}})
	if conversation.Phase() != ConversationRunning || conversation.SegmentID() != "seg_2" || len(conversation.Interactions()) != 0 {
		t.Fatalf("resumed projection = phase %v, segment %q", conversation.Phase(), conversation.SegmentID())
	}
	completedTool := startedApprovalTool.Clone()
	completedTool.Status = ToolOK
	apply(t, conversation, RunEvent{EventID: "approval-done", RunID: "run_1", SegmentID: "seg_2", Event: BlockCompleted{Block: Block{
		ID: approval.ItemID, RunID: "run_1", Status: BlockStatusCompleted, Kind: BlockTool, Tool: &completedTool,
	}}})
	finalContext := int64(4_096)
	apply(t, conversation, RunEvent{EventID: "different-space:done", RunID: "run_1", SegmentID: "seg_2", Event: RunFinished{
		Outcome: Outcome{Status: OutcomeCompleted}, Usage: Usage{InputTokens: 14, OutputTokens: 4}, ContextTokens: finalContext,
	}})
	if conversation.Phase() != ConversationIdle || conversation.Outcome().Status != OutcomeCompleted {
		t.Fatalf("terminal projection = phase %v, outcome %+v", conversation.Phase(), conversation.Outcome())
	}
	if blocks := conversation.Blocks(); len(blocks) != 3 || blocks[0].Text != "final" {
		t.Fatalf("blocks = %+v", blocks)
	}
	if runs := conversation.Runs(); len(runs) != 1 || runs[0].ContextTokens != finalContext {
		t.Fatalf("finished run context = %+v, want %d", runs, finalContext)
	}
	if current, ok := conversation.CurrentRun(); !ok || current.Status != protocol.RunStatusFinished || current.ContextTokens != finalContext {
		t.Fatalf("finished current Run = %+v, %t", current, ok)
	}
}

func TestConversationAppendsTextDeltasInEventOrder(t *testing.T) {
	conversation := NewConversation()
	run := runningRun("seg_1")
	apply(t, conversation, RunEvent{EventID: "start", RunID: run.ID, SegmentID: run.ActiveSegmentID, Event: SegmentStarted{Run: run}})
	for _, block := range []Block{
		{ID: "answer", RunID: run.ID, Status: BlockStatusRunning, Kind: BlockAssistant},
		{ID: "reasoning", RunID: run.ID, Status: BlockStatusRunning, Kind: BlockReasoning},
		{ID: "tool", RunID: run.ID, Status: BlockStatusRunning, Kind: BlockTool, Tool: &ToolCall{Kind: ToolShell, Name: "shell", Status: ToolRunning}},
	} {
		apply(t, conversation, RunEvent{EventID: "start-" + block.ID, RunID: run.ID, SegmentID: run.ActiveSegmentID, Event: BlockStarted{Block: block}})
	}

	apply(t, conversation, RunEvent{EventID: "answer-first", RunID: run.ID, SegmentID: run.ActiveSegmentID, Event: BlockDelta{BlockID: "answer", Text: "first"}})
	apply(t, conversation, RunEvent{EventID: "answer-second", RunID: run.ID, SegmentID: run.ActiveSegmentID, Event: BlockDelta{BlockID: "answer", Text: " second"}})
	blocks := conversation.Blocks()
	if blocks[0].Text != "first second" {
		t.Fatalf("assistant text = %q", blocks[0].Text)
	}

	apply(t, conversation, RunEvent{EventID: "answer-complete", RunID: run.ID, SegmentID: run.ActiveSegmentID, Event: BlockCompleted{Block: Block{
		ID: "answer", RunID: run.ID, Status: BlockStatusCompleted, Kind: BlockAssistant, Text: "authoritative",
	}}})
	if got := conversation.Blocks()[0].Text; got != "authoritative" {
		t.Fatalf("completed assistant text = %q", got)
	}
}

func TestConversationRejectsRegressingRunUsage(t *testing.T) {
	conversation := NewConversation()
	run := runningRun("seg_1")
	run.Usage = Usage{InputTokens: 10}
	apply(t, conversation, RunEvent{EventID: "start", RunID: "run_1", SegmentID: "seg_1", Event: SegmentStarted{Run: run}})
	approval := runningApproval("approval_1", "shell")
	apply(t, conversation, RunEvent{EventID: "approval", RunID: "run_1", SegmentID: "seg_1", Event: BlockStarted{Block: Block{
		ID: approval.ItemID, RunID: "run_1", Status: BlockStatusRunning, Kind: BlockTool, Tool: approval.Tool,
	}}})
	interrupted := RunInterrupted{
		Interactions: []Interaction{approval},
		Usage:        Usage{InputTokens: 9},
	}
	if _, err := conversation.ApplyRunEvent(RunEvent{EventID: "wait", RunID: "run_1", SegmentID: "seg_1", Event: interrupted}); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("regressing usage error = %v", err)
	}
}

func TestConversationRejectsApprovalForDifferentToolInvocation(t *testing.T) {
	conversation := NewConversation()
	run := runningRun("seg_1")
	apply(t, conversation, RunEvent{EventID: "start", RunID: run.ID, SegmentID: run.ActiveSegmentID, Event: SegmentStarted{Run: run}})

	approval := runningApproval("approval_1", "shell")
	approval.Tool.Command = "echo approved"
	approval.Tool.ArgumentsJSON = []byte(`{"command":"echo approved"}`)
	startedTool := approval.Tool.Clone()
	startedTool.Command = "echo different"
	startedTool.ArgumentsJSON = []byte(`{"command":"echo different"}`)
	apply(t, conversation, RunEvent{EventID: "approval", RunID: run.ID, SegmentID: run.ActiveSegmentID, Event: BlockStarted{Block: Block{
		ID: approval.ItemID, RunID: run.ID, Status: BlockStatusRunning, Kind: BlockTool, Tool: &startedTool,
	}}})

	_, err := conversation.ApplyRunEvent(RunEvent{
		EventID: "wait", RunID: run.ID, SegmentID: run.ActiveSegmentID,
		Event: RunInterrupted{Interactions: []Interaction{approval}, Usage: run.Usage},
	})
	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("different approval invocation error = %v", err)
	}
}

func TestConversationFoldsRunProgressWithoutMakingPreviewsDurable(t *testing.T) {
	conversation := NewConversation()
	cost := 0.1
	run := runningRun("seg_1")
	run.Usage = Usage{
		InputTokens: 10, CostUSD: &cost, Steps: 2,
		ByModel: map[string]ModelUsage{"mock/balanced": {InputTokens: 10}},
	}
	apply(t, conversation, RunEvent{EventID: "start", RunID: run.ID, SegmentID: run.ActiveSegmentID, Event: SegmentStarted{Run: run}})

	progressUsage := Usage{
		InputTokens:  14,
		OutputTokens: 2,
		ByModel:      map[string]ModelUsage{"mock/balanced": {InputTokens: 14, OutputTokens: 2}},
	}
	step := 3
	contextTokens := int64(16_384)
	apply(t, conversation, RunEvent{EventID: "progress", RunID: run.ID, SegmentID: run.ActiveSegmentID, Event: RunProgress{
		Step: &step, Usage: &progressUsage, ContextTokens: &contextTokens, Activity: "thinking",
	}})
	got := conversation.Usage()
	if got.InputTokens != 14 || got.OutputTokens != 2 || got.CostUSD != nil ||
		got.Steps != step || got.ByModel["mock/balanced"].InputTokens != 14 {
		t.Fatalf("progress usage = %+v", got)
	}
	if conversation.Checkpoint() != "start" {
		t.Fatalf("ephemeral progress advanced checkpoint to %q", conversation.Checkpoint())
	}
	compactedContext := int64(4_096)
	apply(t, conversation, RunEvent{EventID: "context-after-compaction", RunID: run.ID, SegmentID: run.ActiveSegmentID, Event: RunProgress{
		ContextTokens: &compactedContext,
	}})
	if runs := conversation.Runs(); len(runs) != 1 || runs[0].ContextTokens != compactedContext || runs[0].Usage.Steps != step {
		t.Fatalf("context-only progress did not update the run: %+v", runs)
	}

	regressed := Usage{InputTokens: 13, OutputTokens: 2}
	invalidContext := int64(99_999)
	if _, err := conversation.ApplyRunEvent(RunEvent{EventID: "regression", RunID: run.ID, SegmentID: run.ActiveSegmentID, Event: RunProgress{
		Usage: &regressed, ContextTokens: &invalidContext,
	}}); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("regressing progress error = %v", err)
	}
	if got := conversation.Runs()[0].ContextTokens; got != compactedContext {
		t.Fatalf("rejected progress changed context tokens to %d", got)
	}
}

func TestConversationValidatesEphemeralToolAndCustomEventsAgainstTheActiveRun(t *testing.T) {
	conversation := NewConversation()
	run := runningRun("seg_1")
	apply(t, conversation, RunEvent{EventID: "start", RunID: run.ID, SegmentID: run.ActiveSegmentID, Event: SegmentStarted{Run: run}})
	apply(t, conversation, RunEvent{EventID: "tool", RunID: run.ID, SegmentID: run.ActiveSegmentID, Event: BlockStarted{Block: Block{
		ID: "tool_1", RunID: run.ID, Status: BlockStatusRunning, Kind: BlockTool,
		Tool: &ToolCall{Kind: ToolShell, Name: "shell", Status: ToolRunning},
	}}})
	apply(t, conversation, RunEvent{EventID: "args", RunID: run.ID, SegmentID: run.ActiveSegmentID, Event: ToolArgumentsDelta{BlockID: "tool_1", Text: `{"command":"go test`}})
	apply(t, conversation, RunEvent{EventID: "custom", RunID: run.ID, SegmentID: run.ActiveSegmentID, Event: CustomEvent{Name: "vendor.trace", PayloadJSON: []byte(`{"span":"abc"}`)}})
	if conversation.Checkpoint() != "tool" {
		t.Fatalf("ephemeral events advanced checkpoint to %q", conversation.Checkpoint())
	}
	if _, err := conversation.ApplyRunEvent(RunEvent{EventID: "orphan", RunID: run.ID, SegmentID: run.ActiveSegmentID, Event: ToolArgumentsDelta{BlockID: "missing"}}); !errors.Is(err, ErrUnknownBlock) {
		t.Fatalf("orphan tool arguments error = %v", err)
	}
}

func TestConversationFoldsAChildRunWithoutEndingTheRootStream(t *testing.T) {
	conversation := NewConversation()
	if err := conversation.Starting(); err != nil {
		t.Fatal(err)
	}
	root := runningRun("seg_root")
	apply(t, conversation, treeEvent("open-root", root.ID, root.ActiveSegmentID, root.ActiveSegmentID, SegmentStarted{Run: root}))
	delegate := Block{
		ID: "delegate", RunID: root.ID, Status: BlockStatusRunning, Kind: BlockTool,
		Tool: &ToolCall{Kind: ToolTask, Name: "delegate_task", Status: ToolRunning},
	}
	apply(t, conversation, treeEvent("delegate-start", root.ID, root.ActiveSegmentID, root.ActiveSegmentID, BlockStarted{Block: delegate}))

	child := runningRun("seg_child")
	child.ID = "run_child"
	child.Lineage = testChildRunLineage(t, child.ID, delegate.ID, root.ID, root.ID)
	apply(t, conversation, treeEvent("open-child", child.ID, child.ActiveSegmentID, root.ActiveSegmentID, SegmentStarted{Run: child}))
	if got := conversation.RunningDescendants(); got != 1 {
		t.Fatalf("running descendants after child start = %d, want 1", got)
	}
	childAnswer := Block{ID: "child-answer", RunID: child.ID, Status: BlockStatusRunning, Kind: BlockAssistant}
	apply(t, conversation, treeEvent("child-answer-start", child.ID, child.ActiveSegmentID, root.ActiveSegmentID, BlockStarted{Block: childAnswer}))
	apply(t, conversation, treeEvent("child-answer-delta", child.ID, child.ActiveSegmentID, root.ActiveSegmentID, BlockDelta{BlockID: childAnswer.ID, Text: "inspection"}))
	childAnswer.Status, childAnswer.Text = BlockStatusCompleted, "inspection complete"
	apply(t, conversation, treeEvent("child-answer-done", child.ID, child.ActiveSegmentID, root.ActiveSegmentID, BlockCompleted{Block: childAnswer}))
	apply(t, conversation, treeEvent("child-done", child.ID, child.ActiveSegmentID, root.ActiveSegmentID, RunFinished{Outcome: Outcome{Status: OutcomeCompleted}, Usage: Usage{InputTokens: 4}}))
	if got := conversation.RunningDescendants(); got != 0 {
		t.Fatalf("running descendants after child finish = %d, want 0", got)
	}
	if conversation.Phase() != ConversationRunning || conversation.RunID() != root.ID {
		t.Fatalf("child terminal ended root conversation: phase=%v run=%s", conversation.Phase(), conversation.RunID())
	}

	delegate.Status = BlockStatusCompleted
	delegate.Tool.Status = ToolOK
	apply(t, conversation, treeEvent("delegate-done", root.ID, root.ActiveSegmentID, root.ActiveSegmentID, BlockCompleted{Block: delegate}))
	apply(t, conversation, treeEvent("root-done", root.ID, root.ActiveSegmentID, root.ActiveSegmentID, RunFinished{Outcome: Outcome{Status: OutcomeCompleted}, Usage: Usage{InputTokens: 8}}))
	if conversation.Phase() != ConversationIdle || conversation.Outcome().Status != OutcomeCompleted {
		t.Fatalf("root terminal projection = phase %v outcome %+v", conversation.Phase(), conversation.Outcome())
	}
	if blocks := conversation.Blocks(); len(blocks) != 2 || blocks[1].RunID != child.ID || blocks[1].Text != "inspection complete" {
		t.Fatalf("tree transcript = %+v", blocks)
	}
}

func TestConversationResumesATreeInterruptedByAChild(t *testing.T) {
	conversation := NewConversation()
	root := runningRun("seg_root_1")
	apply(t, conversation, treeEvent("root-start-1", root.ID, root.ActiveSegmentID, root.ActiveSegmentID, SegmentStarted{Run: root}))
	delegate := Block{
		ID: "delegate", RunID: root.ID, Status: BlockStatusRunning, Kind: BlockTool,
		Tool: &ToolCall{Kind: ToolTask, Name: "delegate_task", Status: ToolRunning},
	}
	apply(t, conversation, treeEvent("delegate-start", root.ID, root.ActiveSegmentID, root.ActiveSegmentID, BlockStarted{Block: delegate}))
	child := runningRun("seg_child_1")
	child.ID = "run_child"
	child.Lineage = testChildRunLineage(t, child.ID, delegate.ID, root.ID, root.ID)
	apply(t, conversation, treeEvent("child-start-1", child.ID, child.ActiveSegmentID, root.ActiveSegmentID, SegmentStarted{Run: child}))
	approval := Approval{
		RunID: child.ID, ItemID: "child-approval", Title: "Inspect generated output",
		Tool: &ToolCall{Kind: ToolRead, Name: "read", Status: ToolRunning},
	}
	apply(t, conversation, treeEvent("approval-start", child.ID, child.ActiveSegmentID, root.ActiveSegmentID, BlockStarted{Block: Block{
		ID: approval.ItemID, RunID: child.ID, Status: BlockStatusRunning, Kind: BlockTool, Tool: approval.Tool,
	}}))
	apply(t, conversation, treeEvent("child-wait", child.ID, child.ActiveSegmentID, root.ActiveSegmentID, RunInterrupted{Interactions: []Interaction{approval}, Usage: Usage{InputTokens: 3}}))
	apply(t, conversation, treeEvent("root-suspend", root.ID, root.ActiveSegmentID, root.ActiveSegmentID, RunSuspended{Usage: Usage{InputTokens: 5}}))
	if conversation.Phase() != ConversationWaiting || len(conversation.Interactions()) != 1 || conversation.Interactions()[0].(Approval).RunID != child.ID {
		t.Fatalf("tree wait = phase %v interactions %+v", conversation.Phase(), conversation.Interactions())
	}

	resumedRoot := root
	resumedRoot.ActiveSegmentID = "seg_root_2"
	resumedRoot.Usage = Usage{InputTokens: 5}
	resumedChild := child
	resumedChild.ActiveSegmentID = "seg_child_2"
	resumedChild.Usage = Usage{InputTokens: 3}
	apply(t, conversation, treeEvent("child-start-2", child.ID, resumedChild.ActiveSegmentID, resumedRoot.ActiveSegmentID, SegmentStarted{Run: resumedChild}))
	apply(t, conversation, treeEvent("root-start-2", root.ID, resumedRoot.ActiveSegmentID, resumedRoot.ActiveSegmentID, SegmentStarted{Run: resumedRoot}))
	completedApproval := approval.Tool.Clone()
	completedApproval.Status = ToolOK
	apply(t, conversation, treeEvent("approval-done", child.ID, resumedChild.ActiveSegmentID, resumedRoot.ActiveSegmentID, BlockCompleted{Block: Block{
		ID: approval.ItemID, RunID: child.ID, Status: BlockStatusCompleted, Kind: BlockTool, Tool: &completedApproval,
	}}))
	apply(t, conversation, treeEvent("child-done", child.ID, resumedChild.ActiveSegmentID, resumedRoot.ActiveSegmentID, RunFinished{Outcome: Outcome{Status: OutcomeCompleted}, Usage: Usage{InputTokens: 4}}))
	delegate.Status, delegate.Tool.Status = BlockStatusCompleted, ToolOK
	apply(t, conversation, treeEvent("delegate-done", root.ID, resumedRoot.ActiveSegmentID, resumedRoot.ActiveSegmentID, BlockCompleted{Block: delegate}))
	apply(t, conversation, treeEvent("root-done", root.ID, resumedRoot.ActiveSegmentID, resumedRoot.ActiveSegmentID, RunFinished{Outcome: Outcome{Status: OutcomeCompleted}, Usage: Usage{InputTokens: 9}}))
	if conversation.Phase() != ConversationIdle || conversation.SegmentID() != resumedRoot.ActiveSegmentID {
		t.Fatalf("resumed tree = phase %v segment %s", conversation.Phase(), conversation.SegmentID())
	}
}

func treeEvent(eventID, runID, segmentID, streamSegmentID string, event Event) RunEvent {
	return RunEvent{
		EventID: eventID, RunID: runID, SegmentID: segmentID,
		StreamSegmentID: streamSegmentID, Event: event,
	}
}

func TestConversationTreatsEventIDAsOpaqueIdentity(t *testing.T) {
	conversation := NewConversation()
	event := RunEvent{EventID: "z/not-a-number", RunID: "run_1", SegmentID: "seg_1", Event: SegmentStarted{Run: runningRun("seg_1")}}
	apply(t, conversation, event)
	accepted, err := conversation.ApplyRunEvent(event.Clone())
	if err != nil || accepted.Applied {
		t.Fatalf("identical replay = %+v, %v", accepted, err)
	}
	conflict := event
	conflict.Event = SegmentStarted{Run: testRootRun(Run{ID: "run_1", SessionID: "ses_1", Status: protocol.RunStatusRunning, ActiveSegmentID: "seg_1", Provider: "mock", Model: "other"})}
	if _, err := conversation.ApplyRunEvent(conflict); !errors.Is(err, ErrEventConflict) {
		t.Fatalf("conflict error = %v", err)
	}
}

func TestConversationRejectsCrossSegmentAndInvalidTransitions(t *testing.T) {
	conversation := NewConversation()
	apply(t, conversation, RunEvent{EventID: "start", RunID: "run_1", SegmentID: "seg_1", Event: SegmentStarted{Run: runningRun("seg_1")}})
	_, err := conversation.ApplyRunEvent(RunEvent{EventID: "wrong", RunID: "run_1", SegmentID: "seg_2", Event: BlockCompleted{Block: Block{ID: "x", RunID: "run_1", Status: BlockStatusCompleted, Kind: BlockAssistant, Text: "x"}}})
	if !errors.Is(err, ErrInvalidTransition) && err == nil {
		t.Fatal("cross-segment event was accepted")
	}
	_, err = conversation.ApplyRunEvent(RunEvent{EventID: "finish", RunID: "run_1", SegmentID: "seg_1", Event: RunFinished{Outcome: Outcome{Status: OutcomeCompleted}}})
	if err != nil {
		t.Fatal(err)
	}
}

func TestConversationStartingWindow(t *testing.T) {
	conversation := NewConversation()
	if err := conversation.Starting(); err != nil {
		t.Fatal(err)
	}
	if err := conversation.Starting(); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("second Starting error = %v", err)
	}
	if err := conversation.CancelStarting(); err != nil {
		t.Fatal(err)
	}
	if conversation.Outcome().Status != OutcomeCanceled {
		t.Fatalf("outcome = %+v", conversation.Outcome())
	}
}

func TestConversationSettlesRunningItemsWithOutOfBandCancellation(t *testing.T) {
	conversation := NewConversation()
	apply(t, conversation, RunEvent{EventID: "start", RunID: "run_1", SegmentID: "seg_1", Event: SegmentStarted{Run: runningRun("seg_1")}})
	apply(t, conversation, RunEvent{EventID: "tool", RunID: "run_1", SegmentID: "seg_1", Event: BlockStarted{Block: Block{
		ID: "tool_1", RunID: "run_1", Status: BlockStatusRunning, Kind: BlockTool,
		Tool: &ToolCall{Kind: ToolShell, Name: "shell", Status: ToolRunning},
	}}})
	if err := conversation.SettleRun(testRootRun(Run{ID: "run_1", SessionID: "ses_1", Status: protocol.RunStatusFinished, Outcome: Outcome{Status: OutcomeCanceled}})); err != nil {
		t.Fatal(err)
	}
	block := conversation.Blocks()[0]
	if block.Status != BlockStatusIncomplete || block.Tool.Status != ToolCanceled {
		t.Fatalf("settled block = %+v", block)
	}
}

func TestConversationReconcilesAttachThenReadOverlap(t *testing.T) {
	conversation := NewConversation()
	snapshot := attachedReconciliationSnapshot(t)
	plan := snapshot.Plan.State.Steps
	stream := SegmentStream{RunID: "run_1", SegmentID: "seg_1", HeadEventID: "head", Events: func(func(RunEvent, error) bool) {}}
	if err := conversation.RestoreAttachedSnapshot(snapshot, stream); err != nil {
		t.Fatal(err)
	}
	ignored := []RunEvent{
		{EventID: "overlap-start", RunID: "run_1", SegmentID: "seg_1", Event: BlockStarted{Block: Block{ID: "same", RunID: "run_1", Status: BlockStatusRunning, Kind: BlockAssistant}}},
		{EventID: "overlap-delta", RunID: "run_1", SegmentID: "seg_1", Event: BlockDelta{BlockID: "same", Text: "duplicate preview"}},
		{EventID: "overlap-complete", RunID: "run_1", SegmentID: "seg_1", Event: BlockCompleted{Block: snapshot.Transcript[1]}},
	}
	for _, event := range ignored {
		accepted, err := conversation.ApplyRunEvent(event)
		if err != nil {
			t.Fatalf("apply overlap %s: %v", event.EventID, err)
		}
		if accepted.Applied {
			t.Fatalf("overlap %s was folded twice", event.EventID)
		}
	}
	startConflict := RunEvent{
		EventID: "overlap-start-conflict", RunID: "run_1", SegmentID: "seg_1",
		Event: BlockStarted{Block: Block{ID: "same", RunID: "run_1", Status: BlockStatusRunning, Kind: BlockReasoning}},
	}
	if _, err := conversation.ApplyRunEvent(startConflict); !errors.Is(err, ErrEventConflict) {
		t.Fatalf("replayed start conflict = %v", err)
	}
	// A non-overlapping event may be published before an already-committed Plan
	// event. It must not erase the Plan's independent cold-read watermark.
	apply(t, conversation, RunEvent{
		EventID: "new-progress", RunID: "run_1", SegmentID: "seg_1",
		Event: RunProgress{Usage: &Usage{}},
	})
	currentPlan := testPlanChanged(t, 2, plan)
	currentPlan.Plan.State.UpdatedAt = currentPlan.Plan.State.UpdatedAt.Add(time.Minute)
	for _, event := range []RunEvent{
		{EventID: "overlap-plan-old", RunID: "run_1", SegmentID: "seg_1", Event: testPlanChanged(t, 1, []protocol.PlanStep{{Description: "older", Status: protocol.PlanStatusPending}})},
		{EventID: "overlap-plan-current", RunID: "run_1", SegmentID: "seg_1", Event: currentPlan},
	} {
		accepted, err := conversation.ApplyRunEvent(event)
		if err != nil || accepted.Applied {
			t.Fatalf("apply Plan overlap %s = %+v, %v", event.EventID, accepted, err)
		}
	}
	conflict := RunEvent{EventID: "overlap-plan-conflict", RunID: "run_1", SegmentID: "seg_1", Event: testPlanChanged(t, 2, []protocol.PlanStep{{Description: "different", Status: protocol.PlanStatusInProgress}})}
	if _, err := conversation.ApplyRunEvent(conflict); !errors.Is(err, ErrEventConflict) {
		t.Fatalf("same-revision plan conflict = %v", err)
	}

	accepted, err := conversation.ApplyRunEvent(RunEvent{EventID: "new-delta", RunID: "run_1", SegmentID: "seg_1", Event: BlockDelta{BlockID: "live", Text: "preview"}})
	if err != nil || !accepted.Applied {
		t.Fatalf("new live delta = %+v, %v", accepted, err)
	}
	accepted, err = conversation.ApplyRunEvent(RunEvent{EventID: "orphan-preview", RunID: "run_1", SegmentID: "seg_1", Event: BlockDelta{BlockID: "missing", Text: "preview without its transient start"}})
	if err != nil || accepted.Applied {
		t.Fatalf("cold-tail orphan preview = %+v, %v", accepted, err)
	}
	completedTool := snapshot.Transcript[2].Tool.Clone()
	completedTool.Status = ToolOK
	apply(t, conversation, RunEvent{EventID: "live-complete", RunID: "run_1", SegmentID: "seg_1", Event: BlockCompleted{Block: Block{ID: "live", RunID: "run_1", Status: BlockStatusCompleted, Kind: BlockTool, Tool: &completedTool}}})
	apply(t, conversation, RunEvent{EventID: "missing-complete", RunID: "run_1", SegmentID: "seg_1", Event: BlockCompleted{Block: Block{ID: "missing", RunID: "run_1", Status: BlockStatusCompleted, Kind: BlockAssistant, Text: "authoritative"}}})
	apply(t, conversation, RunEvent{EventID: "new-plan", RunID: "run_1", SegmentID: "seg_1", Event: testPlanChanged(t, 3, []protocol.PlanStep{{Description: "done", Status: protocol.PlanStatusCompleted}})})
	if conversation.Plan() == nil || conversation.Plan().State.Revision != 3 || conversation.Checkpoint() != "new-plan" {
		t.Fatalf("reconciled state = plan %+v, checkpoint %q", conversation.Plan(), conversation.Checkpoint())
	}
	fence := RunEvent{EventID: "plan-fence", RunID: "run_1", SegmentID: "seg_1", Event: testPlanChanged(t, 3, []protocol.PlanStep{{Description: "done", Status: protocol.PlanStatusCompleted}})}
	accepted, err = conversation.ApplyRunEvent(fence)
	if err != nil || accepted.Applied {
		t.Fatalf("final Plan fence = %+v, %v", accepted, err)
	}
	conflictingFence := RunEvent{EventID: "plan-fence-conflict", RunID: "run_1", SegmentID: "seg_1", Event: testPlanChanged(t, 3, []protocol.PlanStep{{Description: "different", Status: protocol.PlanStatusCompleted}})}
	if _, err := conversation.ApplyRunEvent(conflictingFence); !errors.Is(err, ErrEventConflict) {
		t.Fatalf("conflicting final Plan fence = %v", err)
	}
}

func TestConversationRejectsPlanFromAnotherSession(t *testing.T) {
	conversation := NewConversation()
	apply(t, conversation, RunEvent{
		EventID: "start", RunID: "run_1", SegmentID: "seg_1",
		Event: SegmentStarted{Run: runningRun("seg_1")},
	})
	changed := testPlanChanged(t, 1, []protocol.PlanStep{{Description: "inspect", Status: protocol.PlanStatusInProgress}})
	changed.Plan.SessionID = "ses_other"
	_, err := conversation.ApplyRunEvent(RunEvent{
		EventID: "foreign-plan", RunID: "run_1", SegmentID: "seg_1", Event: changed,
	})
	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("foreign Plan error = %v, want ErrInvalidTransition", err)
	}
}

func attachedReconciliationSnapshot(t testing.TB) SessionSnapshot {
	return SessionSnapshot{
		Session: Session{ID: "ses_1", Status: protocol.SessionStatusRunning, Provider: testSessionProvider, Model: testSessionModel, Workspace: testWorkspace("/tmp/demo"), Revision: 1},
		Transcript: []Block{
			{ID: "same", RunID: "run_old", Status: BlockStatusCompleted, Kind: BlockAssistant, Text: "old"},
			{ID: "same", RunID: "run_1", Status: BlockStatusCompleted, Kind: BlockAssistant, Text: "current"},
			{ID: "live", RunID: "run_1", Status: BlockStatusRunning, Kind: BlockTool, Tool: &ToolCall{Kind: ToolShell, Name: "shell", Status: ToolRunning}},
		},
		Runs: []Run{
			testRootRun(Run{ID: "run_old", SessionID: "ses_1", Status: protocol.RunStatusFinished, Outcome: Outcome{Status: OutcomeCompleted}}),
			testRootRun(Run{ID: "run_1", SessionID: "ses_1", Status: protocol.RunStatusRunning, ActiveSegmentID: "seg_1"}),
		},
		Plan: testPlan(t, 2, []protocol.PlanStep{{Description: "inspect", Status: protocol.PlanStatusInProgress}}),
	}
}

func TestConversationRejectsOrphanPreviewOutsideColdTail(t *testing.T) {
	conversation := NewConversation()
	apply(t, conversation, RunEvent{EventID: "start", RunID: "run_1", SegmentID: "seg_1", Event: SegmentStarted{Run: runningRun("seg_1")}})
	if _, err := conversation.ApplyRunEvent(RunEvent{
		EventID: "orphan", RunID: "run_1", SegmentID: "seg_1",
		Event: BlockDelta{BlockID: "missing", Text: "bad"},
	}); !errors.Is(err, ErrUnknownBlock) {
		t.Fatalf("orphan preview error = %v", err)
	}
}

func runningRun(segmentID string) Run {
	return testRootRun(Run{ID: "run_1", SessionID: "ses_1", Provider: "mock", Model: "balanced", Status: protocol.RunStatusRunning, ActiveSegmentID: segmentID})
}

func apply(t *testing.T, conversation *Conversation, event RunEvent) {
	t.Helper()
	accepted, err := conversation.ApplyRunEvent(event)
	if err != nil {
		t.Fatal(err)
	}
	if !accepted.Applied {
		t.Fatal("event was not applied")
	}
}

func TestBlockIdentityKeyPreservesFieldBoundaries(t *testing.T) {
	left := (BlockIdentity{RunID: "a", BlockID: "bc"}).Key()
	right := (BlockIdentity{RunID: "ab", BlockID: "c"}).Key()
	if left == right {
		t.Fatal("different block identity fields produced the same terminal key")
	}
}
