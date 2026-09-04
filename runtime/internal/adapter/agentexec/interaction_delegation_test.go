package agentexec

import (
	"context"
	"errors"
	"iter"
	"math"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/application/ownership"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	agent "github.com/Tangerg/scope/agent"
	"github.com/Tangerg/scope/agent/interaction"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient"
)

func uint32Pointer(value uint32) *uint32 { return &value }
func uint64Pointer(value uint64) *uint64 { return &value }
func intPointer(value int) *int          { return &value }
func durationPointer(value time.Duration) *time.Duration {
	return &value
}

func TestDelegatedInteractionReplyPreservesRefusal(t *testing.T) {
	message := chat.NewAssistantMessage(chat.NewRefusalPart("I cannot complete that delegated task."))
	response := chat.Response{Output: &chat.Output{
		Message: &message, FinishReason: chat.FinishReasonRefusal,
	}}

	reply, err := delegatedInteractionReply(interaction.Output{
		Source: interaction.CompletionSourceModelResponse, ModelResponse: &response, ModelCalls: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if reply != "I cannot complete that delegated task." {
		t.Fatalf("delegated reply = %q", reply)
	}
}

func TestInteractionDelegationPolicyPreservesOptionalPresence(t *testing.T) {
	defaults, err := effectiveDelegation(InteractionDelegationPolicyValues{})
	if err != nil {
		t.Fatal(err)
	}
	if defaults.treeLimits != (agent.TreeLimits{
		MaxDepth: defaultDelegateDepth, MaxChildren: defaultDelegateChildren,
		MaxActiveChildren: defaultActiveDelegateChildren, MaxTreeProcesses: defaultDelegateTreeProcesses,
	}) || defaults.processBudget != (agent.Budget{
		Steps: defaultDelegateSteps, Effects: defaultDelegateEffects, Signals: defaultDelegateSignals,
	}) {
		t.Fatalf("default delegation policy = %+v", defaults)
	}

	if _, err := effectiveDelegation(InteractionDelegationPolicyValues{MaxDepth: uint32Pointer(0)}); err == nil {
		t.Fatal("present zero tree limit was treated as omission")
	}
	if _, err := effectiveDelegation(InteractionDelegationPolicyValues{ChildSteps: uint64Pointer(0)}); err == nil {
		t.Fatal("present zero child budget was treated as omission")
	}
}

func TestDelegateSubtreeBudgetReservesEveryRemainingProcessLevel(t *testing.T) {
	base := agent.Budget{Steps: 2, Effects: 3, Signals: 5}
	budget, err := delegateSubtreeBudget(base, 4)
	if err != nil {
		t.Fatal(err)
	}
	if budget != (agent.Budget{Steps: 8, Effects: 12, Signals: 20}) {
		t.Fatalf("scaled budget = %+v", budget)
	}
	if _, err := delegateSubtreeBudget(base, 0); err == nil {
		t.Fatal("zero process levels were accepted")
	}
	if _, err := delegateSubtreeBudget(
		agent.Budget{Steps: math.MaxUint64, Effects: 1, Signals: 1}, 2,
	); err == nil {
		t.Fatal("overflowing delegated subtree budget was accepted")
	}
}

func TestInteractionExecutorRunsDelegateAsProductChildRun(t *testing.T) {
	model := newDelegatingStubModel()
	client, err := chatclient.New(model, chatclient.Config{})
	if err != nil {
		t.Fatal(err)
	}
	executor, err := NewInteractionExecutor(InteractionExecutorConfig{
		Lifetime:               t.Context(),
		ChatResolver:           staticInteractionChatResolver(client),
		ImplementationIdentity: "interaction-delegate-test-build",
		ConfigurationIdentity:  "interaction-delegate-test-config", DefaultMaxModelCalls: uint32Pointer(4),
		BuildID: interactionTestBuildID,
	})
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	sessions := &delegateSessionStore{value: testsupport.MustRestoreSession(session.Snapshot{
		ID: "session_1", Title: "delegate", Workspace: testsupport.MustWorkspace(workspace),
	})}
	projection := newDelegateProjection()
	runIDs := []string{"run_root", "run_child"}
	segmentIDs := []string{"segment_root", "segment_child"}
	coordinator := mustNewRunCoordinator(t, runs.Dependencies{
		RootStarts: executor, Observations: executor, Releases: executor,
		Conversation: delegateConversation{},
		Session:      runs.SessionPorts{Reader: sessions, Creator: sessions, ActiveRuns: sessions},
		Projection: runs.ProjectionPorts{
			Openings: projection, ChildStarts: projection, Events: projection,
			Barriers: projection, Checkpoints: projection, Workspace: projection, Finalizer: projection,
		},
		Admissions: new(ownership.Gate), Now: time.Now,
		NewRunID: func() string {
			id := runIDs[0]
			runIDs = runIDs[1:]
			return id
		},
		NewSegmentID: func() string {
			id := segmentIDs[0]
			segmentIDs = segmentIDs[1:]
			return id
		},
	})
	started, err := coordinator.Start(t.Context(), runs.StartCommand{
		SessionID:    "session_1",
		Capabilities: run.Capabilities{ChildRuns: true},
		Input:        []transcript.ContentBlock{{Kind: transcript.TextContent, Text: "please delegate this work"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	events := slices.Collect(started.Events)
	if len(events) == 0 {
		t.Fatal("Delegate produced no Run events")
	}
	childStarted := -1
	childFinished := -1
	parentToolFinished := -1
	rootFinished := -1
	childState := run.Running
	rootState := run.Running
	var childFailure, rootFailure *run.Failure
	for index, event := range events {
		switch payload := event.Payload.(type) {
		case runs.SegmentStarted:
			if event.RunID == "run_child" {
				childStarted = index
			}
		case runs.SegmentFinished:
			if event.RunID == "run_child" {
				childFinished = index
				childState = payload.Run.State()
				if failure, failed := payload.Run.Failure(); failed {
					childFailure = &failure
				}
			}
			if event.RunID == "run_root" {
				rootFinished = index
				rootState = payload.Run.State()
				if failure, failed := payload.Run.Failure(); failed {
					rootFailure = &failure
				}
			}
		case runs.ItemCompleted:
			invocation, present := payload.Item.ToolInvocation()
			if event.RunID == "run_root" && present && invocation.Name == "delegate_task" {
				parentToolFinished = index
			}
		}
	}
	if childStarted < 0 || childFinished <= childStarted || parentToolFinished <= childFinished ||
		rootFinished <= parentToolFinished {
		t.Fatalf(
			"Delegate order child-start=%d child-finish=%d parent-tool=%d root-finish=%d events=%#v",
			childStarted, childFinished, parentToolFinished, rootFinished, events,
		)
	}
	if childState != run.Completed || rootState != run.Completed {
		t.Fatalf(
			"Delegate terminal states child=%s error=%+v root=%s error=%+v",
			childState, childFailure, rootState, rootFailure,
		)
	}
	projection.mu.Lock()
	reservation := projection.reservations
	outcomes := projection.outcomes
	openings := slices.Clone(projection.openings)
	conversation := slices.Clone(projection.conversation)
	projection.mu.Unlock()
	if len(reservation) != 1 || len(outcomes) != 1 || len(openings) != 2 {
		t.Fatalf(
			"Delegate durability reservations=%d outcomes=%d openings=%d",
			len(reservation), len(outcomes), len(openings),
		)
	}
	childOpening, admitted := openings[1].Admission()
	if !admitted || childOpening.RunID != "run_child" ||
		childOpening.ParentRunID != "run_root" ||
		childOpening.SpawnedByItemID == "" {
		t.Fatalf("managed child opening = %#v", openings[1])
	}
	var delegatedResult *chat.ToolResult
	for _, message := range conversation {
		for _, part := range message.Parts {
			if part.Kind == chat.PartToolResult && part.ToolResult != nil &&
				part.ToolResult.Name == "delegate_task" {
				delegatedResult = part.ToolResult
			}
		}
	}
	if delegatedResult == nil || len(delegatedResult.Output.Content) != 0 ||
		string(delegatedResult.Output.Details) != `{"reply":"subtask: result"}` {
		t.Fatalf("durable Delegate model result = %#v, want exact structured child output", delegatedResult)
	}
	coordinator.BeginShutdown()
	if err := coordinator.AwaitShutdown(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestInteractionExecutorCancelsRunningDelegateAndKeepsRootRunning(t *testing.T) {
	model := newCancelableDelegateModel()
	client, err := chatclient.New(model, chatclient.Config{})
	if err != nil {
		t.Fatal(err)
	}
	executor, err := NewInteractionExecutor(InteractionExecutorConfig{
		Lifetime:               t.Context(),
		ChatResolver:           staticInteractionChatResolver(client),
		ImplementationIdentity: "interaction-running-cancel-test-build",
		ConfigurationIdentity:  "interaction-running-cancel-test-config", DefaultMaxModelCalls: uint32Pointer(4),
		BuildID: interactionTestBuildID,
	})
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	sessions := &delegateSessionStore{value: testsupport.MustRestoreSession(session.Snapshot{
		ID: "session_1", Title: "running cancellation", Workspace: testsupport.MustWorkspace(workspace),
	})}
	projection := newDelegateProjection()
	runIDs := []string{"run_root", "run_child"}
	segmentIDs := []string{"segment_root", "segment_child"}
	cancelAccepted := make(chan struct{})
	runningSubtreeCanceler := notifyingRunningSubtreeCanceler{
		inner: executor,
		accepted: func() {
			close(cancelAccepted)
		},
	}
	coordinator := mustNewRunCoordinator(t, runs.Dependencies{
		RootStarts: executor, Observations: executor, Releases: executor,
		Conversation: delegateConversation{}, RunningSubtreeCanceler: runningSubtreeCanceler,
		Session: runs.SessionPorts{
			Reader: sessions, Creator: sessions, ActiveRuns: sessions,
			Interrupts: sessions, Terminations: sessions,
		},
		Projection: runs.ProjectionPorts{
			Openings: projection, ChildStarts: projection, Events: projection,
			Barriers: projection, Checkpoints: projection, Workspace: projection, Finalizer: projection,
		},
		Runs: projection, Items: projection,
		Admissions: new(ownership.Gate), Now: time.Now,
		NewRunID: func() string {
			id := runIDs[0]
			runIDs = runIDs[1:]
			return id
		},
		NewSegmentID: func() string {
			id := segmentIDs[0]
			segmentIDs = segmentIDs[1:]
			return id
		},
	})
	started, err := coordinator.Start(t.Context(), runs.StartCommand{
		SessionID: "session_1", Capabilities: run.Capabilities{ChildRuns: true},
		Input: []transcript.ContentBlock{{Kind: transcript.TextContent, Text: "delegate cancelable work"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	eventsReady := make(chan []runs.Event, 1)
	go func() { eventsReady <- slices.Collect(started.Events) }()
	select {
	case <-model.childCallStarted:
	case <-time.After(3 * time.Second):
		t.Fatal("delegated child did not enter its model call")
	}

	type cancellationResult struct {
		value runs.CancelResult
		err   error
	}
	canceled := make(chan cancellationResult, 1)
	go func() {
		value, cancelErr := coordinator.Cancel(context.Background(), runs.CancelCommand{
			RunID: "run_child", Reason: "caller canceled delegated work", AllowChildRun: true,
		})
		canceled <- cancellationResult{value: value, err: cancelErr}
	}()
	select {
	case <-cancelAccepted:
	case <-time.After(3 * time.Second):
		t.Fatal("running Delegate cancellation was not accepted")
	}
	select {
	case <-model.childCallReturned:
	case <-time.After(3 * time.Second):
		t.Fatal("running Delegate cancellation did not stop its in-flight model call")
	}

	var canceledResult runs.CancelResult
	select {
	case outcome := <-canceled:
		if outcome.err != nil {
			close(model.releaseRootContinuation)
			t.Fatalf("Cancel running Delegate: %v", outcome.err)
		}
		canceledResult = outcome.value
	case <-time.After(3 * time.Second):
		t.Fatal("running Delegate cancellation did not settle")
	}
	assertRunningDelegateCancellationResult(t, canceledResult)
	select {
	case <-model.rootContinuationStarted:
	case <-time.After(3 * time.Second):
		t.Fatal("root did not consume the canceled child result")
	}
	close(model.releaseRootContinuation)
	var events []runs.Event
	select {
	case events = <-eventsReady:
	case <-time.After(3 * time.Second):
		t.Fatal("root did not finish after child cancellation")
	}
	assertRunningDelegateCancellationEvents(t, events)
	coordinator.BeginShutdown()
	if err := coordinator.AwaitShutdown(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func assertRunningDelegateCancellationResult(t *testing.T, result runs.CancelResult) {
	t.Helper()
	if result.Run.ID() != "run_child" || result.Run.State() != run.Canceled ||
		!runHasOutcome(result.Run, run.OutcomeCanceled) ||
		result.Run.Detail() != "caller canceled delegated work" {
		t.Fatalf("canceled child = %+v", result.Run)
	}
	if result.RootRun == nil || result.RootRun.ID() != "run_root" || result.RootRun.State() != run.Running {
		t.Fatalf("root after child cancellation = %+v, want running", result.RootRun)
	}
}

func runHasOutcome(record run.Run, expected run.Outcome) bool {
	outcome, terminal := record.Outcome()
	return terminal && outcome == expected
}

func assertRunningDelegateCancellationEvents(t *testing.T, events []runs.Event) {
	t.Helper()
	childFinished, rootFinished := false, false
	for _, event := range events {
		finished, ok := event.Payload.(runs.SegmentFinished)
		if !ok {
			continue
		}
		if event.RunID == "run_child" {
			childFinished = finished.Run.State() == run.Canceled &&
				finished.Run.Detail() == "caller canceled delegated work"
		}
		if event.RunID == "run_root" {
			rootFinished = finished.Run.State() == run.Completed
		}
	}
	if !childFinished || !rootFinished {
		t.Fatalf("terminal projection child=%t root=%t events=%#v", childFinished, rootFinished, events)
	}
}

func TestInteractionExecutorProjectsConcurrentDelegateSiblingsExactlyOnce(t *testing.T) {
	result := runDelegateTree(t, newOrderedSiblingDelegateModel(), "run siblings", 3)
	rootID := result.rootRunID(t)
	directChildren := 0
	for _, opening := range result.openings {
		admission, admitted := opening.Admission()
		if !admitted || admission.RunID == rootID {
			continue
		}
		if admission.ParentRunID != rootID || admission.RootRunID != rootID {
			t.Fatalf("sibling opening has invalid lineage: %+v", admission)
		}
		directChildren++
	}
	if directChildren != 2 {
		t.Fatalf("direct child openings = %d, want 2", directChildren)
	}
	result.assertAllRunsCompleted(t)
}

type orderedSiblingDelegateModel struct {
	defaults  *chat.Options
	bReturned chan struct{}
	bOnce     sync.Once
}

func newOrderedSiblingDelegateModel() *orderedSiblingDelegateModel {
	return &orderedSiblingDelegateModel{
		defaults:  &chat.Options{Model: "stub-ordered-sibling-delegate"},
		bReturned: make(chan struct{}),
	}
}

func (o *orderedSiblingDelegateModel) DefaultOptions() chat.Options { return *o.defaults }

func (o *orderedSiblingDelegateModel) Call(
	ctx context.Context,
	request *chat.Request,
) (*chat.Response, error) {
	switch {
	case hasToolMessage(request.Messages):
		return interactionUsageTextResponse("root: siblings done", 2, 1), nil
	case userMessagesContain(request.Messages, "sibling A"):
		select {
		case <-o.bReturned:
			return interactionUsageTextResponse("child: sibling A", 2, 1), nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	case userMessagesContain(request.Messages, "sibling B"):
		o.bOnce.Do(func() { close(o.bReturned) })
		return interactionUsageTextResponse("child: sibling B", 2, 1), nil
	case userMessagesContain(request.Messages, "run siblings"):
		return interactionToolBatchResponse([]chat.ToolCall{
			{ID: "delegate_a", Name: "delegate_task", Arguments: `{"summary":"sibling A","instructions":"sibling A"}`},
			{ID: "delegate_b", Name: "delegate_task", Arguments: `{"summary":"sibling B","instructions":"sibling B"}`},
		}, 2, 1), nil
	default:
		return nil, errors.New("unexpected ordered sibling Delegate model context")
	}
}

func (o *orderedSiblingDelegateModel) Stream(
	ctx context.Context,
	request *chat.Request,
) iter.Seq2[*chat.ResponseDelta, error] {
	return testsupport.StreamResponse(o.Call(ctx, request))
}

func TestInteractionExecutorProjectsNestedDelegateLineageExactlyOnce(t *testing.T) {
	result := runDelegateTree(t, newNestedDelegatingStub(), "nested root", 3)
	rootID := result.rootRunID(t)
	children := make(map[string]string)
	for _, opening := range result.openings {
		admission, admitted := opening.Admission()
		if !admitted || admission.RunID == rootID {
			continue
		}
		children[admission.RunID] = admission.ParentRunID
		if admission.RootRunID != rootID {
			t.Fatalf("nested opening root = %q, want %q", admission.RootRunID, rootID)
		}
	}
	if len(children) != 2 {
		t.Fatalf("nested child openings = %v, want child and grandchild", children)
	}
	var childID string
	for runID, parentID := range children {
		if parentID == rootID {
			childID = runID
		}
	}
	if childID == "" {
		t.Fatalf("nested lineage has no direct child of %q: %v", rootID, children)
	}
	grandchildren := 0
	for _, parentID := range children {
		if parentID == childID {
			grandchildren++
		}
	}
	if grandchildren != 1 {
		t.Fatalf("nested lineage = %v, want one grandchild of %q", children, childID)
	}
	result.assertAllRunsCompleted(t)
}

type delegateTreeResult struct {
	events   []runs.Event
	openings []runs.OpeningCommit
}

func runDelegateTree(
	t *testing.T,
	model chat.Model,
	input string,
	wantProcesses int,
) delegateTreeResult {
	t.Helper()
	client, err := chatclient.New(model, chatclient.Config{})
	if err != nil {
		t.Fatal(err)
	}
	executor, err := NewInteractionExecutor(InteractionExecutorConfig{
		Lifetime:               t.Context(),
		ChatResolver:           staticInteractionChatResolver(client),
		ImplementationIdentity: "interaction-delegate-tree-test-build",
		ConfigurationIdentity:  "interaction-delegate-tree-test-config", DefaultMaxModelCalls: uint32Pointer(6),
		MaxConcurrentToolCalls: intPointer(4), BuildID: interactionTestBuildID,
	})
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	sessions := &delegateSessionStore{value: testsupport.MustRestoreSession(session.Snapshot{
		ID: "session_tree", Title: "delegate tree", Workspace: testsupport.MustWorkspace(workspace),
	})}
	projection := newDelegateProjection()
	var identityMu sync.Mutex
	runSequence, segmentSequence := 0, 0
	coordinator := mustNewRunCoordinator(t, runs.Dependencies{
		RootStarts: executor, Observations: executor, Releases: executor,
		Conversation: delegateConversation{},
		Session:      runs.SessionPorts{Reader: sessions, Creator: sessions, ActiveRuns: sessions},
		Projection: runs.ProjectionPorts{
			Openings: projection, ChildStarts: projection, Events: projection,
			Barriers: projection, Checkpoints: projection, Workspace: projection, Finalizer: projection,
		},
		Admissions: new(ownership.Gate), Now: time.Now,
		NewRunID: func() string {
			identityMu.Lock()
			defer identityMu.Unlock()
			runSequence++
			return "run_tree_" + strconv.Itoa(runSequence)
		},
		NewSegmentID: func() string {
			identityMu.Lock()
			defer identityMu.Unlock()
			segmentSequence++
			return "segment_tree_" + strconv.Itoa(segmentSequence)
		},
	})
	started, err := coordinator.Start(t.Context(), runs.StartCommand{
		SessionID: "session_tree", Capabilities: run.Capabilities{ChildRuns: true},
		Input: []transcript.ContentBlock{{Kind: transcript.TextContent, Text: input}},
	})
	if err != nil {
		t.Fatal(err)
	}
	events := slices.Collect(started.Events)
	coordinator.BeginShutdown()
	if err := coordinator.AwaitShutdown(t.Context()); err != nil {
		t.Fatal(err)
	}
	projection.mu.Lock()
	openings := slices.Clone(projection.openings)
	projection.mu.Unlock()
	if len(openings) != wantProcesses {
		t.Fatalf("delegate tree openings = %d, want %d", len(openings), wantProcesses)
	}
	return delegateTreeResult{events: events, openings: openings}
}

func (d delegateTreeResult) rootRunID(t *testing.T) string {
	t.Helper()
	for _, opening := range d.openings {
		if admission, admitted := opening.Admission(); admitted && admission.ParentRunID == "" {
			return admission.RunID
		}
	}
	t.Fatal("delegate tree has no root opening")
	return ""
}

func (d delegateTreeResult) assertAllRunsCompleted(t *testing.T) {
	t.Helper()
	completed := make(map[string]int, len(d.openings))
	for _, event := range d.events {
		finished, ok := event.Payload.(runs.SegmentFinished)
		if !ok {
			continue
		}
		if finished.Run.State() != run.Completed {
			t.Fatalf(
				"Run %q finished as %s outcome=%v detail=%q error=%+v",
				finished.Run.ID(), finished.Run.State(), finished.Run.Snapshot().Outcome, finished.Run.Detail(), finished.Run.Snapshot().Failure,
			)
		}
		completed[finished.Run.ID()]++
	}
	if len(completed) != len(d.openings) {
		t.Fatalf("completed Runs = %v, openings = %d", completed, len(d.openings))
	}
	for runID, count := range completed {
		if count != 1 {
			t.Fatalf("Run %q terminal projections = %d, want 1", runID, count)
		}
	}
}
