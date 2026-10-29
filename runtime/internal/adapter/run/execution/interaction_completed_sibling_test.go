package execution

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	domaintool "github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	agent "github.com/Tangerg/scope/agent"
	"github.com/Tangerg/scope/agent/strategy/interaction"
	"github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

func TestInteractionExecutorRestoresWaitingTreeWithCompletedSibling(t *testing.T) {
	t.Run("same delegate batch", func(t *testing.T) { testWaitingTreeWithCompletedSibling(t, false) })
	t.Run("earlier delegate batch", func(t *testing.T) { testWaitingTreeWithCompletedSibling(t, true) })
}

func testWaitingTreeWithCompletedSibling(t *testing.T, splitBatch bool) {
	t.Helper()
	bStarted := make(chan agent.ProcessID, 1)
	toolStarted := make(chan agent.ProcessID, 1)
	var writes atomic.Int32
	offloads := &recordingToolResultOffloader{store: testTrees(t).stores.ToolResults}
	ordinary, err := toolcontract.NewFunc(toolcontract.FuncConfig{Name: "fixture_store", Description: "Write a known result."}, func(ctx context.Context, input struct {
		Value string `json:"value"`
	}) (string, error) {
		if input.Value != "edited" {
			return "", errors.New("effective arguments were lost")
		}
		writes.Add(1)
		invocation, found := interaction.ToolInvocationFromContext(ctx)
		if !found {
			return "", errors.New("tool invocation missing")
		}
		toolStarted <- invocation.Relation().ProcessID()
		return strings.Repeat("stored data 界", 100), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	releaseA := make(chan struct{})
	var modelCalls atomic.Int32
	model := chat.ModelFunc(func(ctx context.Context, request *chat.Request) (*chat.Response, error) {
		modelCalls.Add(1)
		if request.Options.Temperature == nil || *request.Options.Temperature != 0.37 || request.Options.MaxOutputTokens == nil || *request.Options.MaxOutputTokens != 731 {
			return nil, errors.New("restored child lost generation options")
		}
		switch {
		case hasToolMessage(request.Messages):
			return interactionUsageTextResponse("continued", 2, 1), nil
		case userMessagesContain(request.Messages, "waiting sibling A"):
			select {
			case <-releaseA:
				return interactionToolResponse(chat.ToolCall{ID: "ask_a", Name: "fixture_ask", Arguments: `{}`}, 2, 1), nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		case userMessagesContain(request.Messages, "completed sibling B"):
			invocation, found := interaction.ModelInvocationFromContext(ctx)
			if !found {
				return nil, errors.New("model invocation is missing")
			}
			bStarted <- invocation.Relation().ProcessID()
			return interactionUsageTextResponse("sibling B completed", 2, 1), nil
		default:
			calls := []chat.ToolCall{
				{ID: "ordinary_c", Name: "fixture_store", Arguments: `{"value":"original"}`},
				{ID: "delegate_a", Name: "delegate_task", Arguments: `{"summary":"A","instructions":"waiting sibling A"}`},
				{ID: "delegate_b", Name: "delegate_task", Arguments: `{"summary":"B","instructions":"completed sibling B"}`},
			}
			if splitBatch {
				calls = []chat.ToolCall{calls[2], calls[0], calls[1]}
			}
			return interactionToolBatchResponse(calls, 2, 1), nil
		}
	})
	question := newDelegateQuestionTool(t, "Ask for a value.")
	executor, err := newProductConfiguredInteractionExecutor(t, InteractionExecutorConfig{
		Lifetime: t.Context(), ChatResolver: staticInteractionChatResolver(model),
		MaxConcurrentToolCalls: intPointer(4), BuildID: interactionTestBuildID,
		ToolResolver:    staticInteractionTools{identities: []domaintool.Ref{testsupport.MCPTool(testsupport.UserMCPServer("fixture"), "ask"), testsupport.MCPTool(testsupport.UserMCPServer("fixture"), "store")}, manifest: toolset.Manifest{Visible: []toolcontract.Tool{question, ordinary}}},
		ToolInterpreter: testInteractionToolInterpreter{}, ToolAuthorizer: allowInteractionTools{},
		ToolHooks: siblingEditHook{}, ToolResultStore: offloads,
		ToolResultOffload: ToolResultOffloadPolicyValues{Threshold: intPointer(100), ReaderName: testToolResultReaderName},
	})
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	current := testsupport.MustRestoreSession(session.Snapshot{
		ID: "session_1", Title: "completed sibling", Workspace: testsupport.MustWorkspace(workspace),
	})
	projection := newDelegateProjection(t, current, executor)
	runSequence, segmentSequence := 0, 0
	observations := newRecordingExecutionObserver(executor)
	coordinator := mustNewRunCoordinator(t, runs.Dependencies{
		RootStarts: executor, Observations: observations, Releases: executor, Conversation: projection.history,
		Session: runs.SessionPorts{Reader: projection.sessions, Creator: projection.sessions, ActiveRuns: projection.sessions},
		Projection: runs.ProjectionPorts{
			Openings: projection, ChildStarts: projection, Events: projection, Barriers: projection,
			Checkpoints: projection, Workspace: projection, Finalizer: projection,
		},
		Admissions: projection.admissions, Now: time.Now,
		NewRunID:     func() string { runSequence++; return "run_" + strconv.Itoa(runSequence) },
		NewSegmentID: func() string { segmentSequence++; return "segment_" + strconv.Itoa(segmentSequence) },
	})
	t.Cleanup(func() {
		coordinator.BeginShutdown()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := coordinator.AwaitShutdown(ctx); err != nil {
			t.Error(err)
		}
	})
	started, err := coordinator.Start(t.Context(), runs.StartCommand{
		SessionID: "session_1", Capabilities: run.Capabilities{ChildRuns: true, InterruptKinds: []run.InterruptKind{run.InterruptQuestion}},
		Input:   []transcript.ContentBlock{{Kind: transcript.TextContent, Text: "delegate both siblings"}},
		Options: &chat.Options{Temperature: new(0.37), MaxOutputTokens: new(int64(731))},
	})
	if err != nil {
		t.Fatal(err)
	}
	eventsReady := make(chan []runs.Event, 1)
	go func() { eventsReady <- slices.Collect(started.Events) }()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	var processID agent.ProcessID
	select {
	case processID = <-bStarted:
	case <-ctx.Done():
		t.Fatal("sibling B did not start")
	}
	execution := executor.sessions.snapshot()[0]
	process, found := execution.engine.Process(processID)
	if !found {
		t.Fatal("sibling B is unavailable")
	}
	if _, err := process.Await(ctx); err != nil {
		t.Fatalf("finish sibling B before A asks for input: %v", err)
	}
	var toolProcessID agent.ProcessID
	select {
	case toolProcessID = <-toolStarted:
	case <-ctx.Done():
		t.Fatal("ordinary sibling did not start")
	}
	toolProcess, found := execution.engine.Process(toolProcessID)
	if !found {
		t.Fatal("ordinary sibling is unavailable")
	}
	if _, err := toolProcess.Await(ctx); err != nil {
		t.Fatal(err)
	}
	items, err := projection.stores.Transcript.List(t.Context(), current.ID())
	if err != nil {
		t.Fatal(err)
	}
	var known transcript.Item
	for _, item := range items {
		if invocation, ok := item.ToolInvocation(); ok && invocation.Offload != nil {
			known = item
		}
	}
	knownInvocation, found := known.ToolInvocation()
	if !found || knownInvocation.Arguments.Canonical() != `{"value":"edited"}` || knownInvocation.Offload == nil || offloads.calls.Load() != 1 {
		t.Fatalf("completed tool was not durably projected: %+v", known)
	}
	close(releaseA)
	fixture := &waitingDelegateFixture{executor: executor, coordinator: coordinator, projection: projection}
	barrier := fixture.waitForBarrier(t, 2*time.Second)
	<-eventsReady
	<-observations.completed
	pending := barrier.Pending()
	checkpoint := barrier.Checkpoint()
	if len(checkpoint.ToolResultIDs) != 0 {
		t.Fatalf("checkpoint retained already published result bodies: %v", checkpoint.ToolResultIDs)
	}
	if len(pending.Continuations) != 2 || len(pending.Bindings) != 1 {
		t.Fatalf("waiting tree includes a completed sibling: %+v", pending)
	}
	for _, member := range pending.Continuations {
		if member.MemberID == processID.String() {
			t.Fatal("completed sibling B is a waiting continuation member")
		}
	}
	if err := executor.Release(t.Context(), runs.ExecutorRef{SessionID: barrier.SessionID(), ExecutorID: pending.ExecutorID}); err != nil {
		t.Fatal(err)
	}
	binding := pending.Bindings[0]
	resumeCtx, cancelResume := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancelResume()
	resumed, err := coordinator.Resume(t.Context(), runs.ResumeCommand{
		RunID: pending.RootRunID, CallerCapabilities: run.Capabilities{ChildRuns: true, InterruptKinds: []run.InterruptKind{run.InterruptQuestion}},
		Responses: []runs.ResumeResponse{{ItemID: binding.InterruptItemID, Kind: run.InterruptQuestion, Question: &runs.QuestionResponse{Answers: [][]string{{"chosen"}}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for range resumed.Events {
		}
	}()
	var observed []runs.ExecutorEvent
	select {
	case observed = <-observations.completed:
	case <-resumeCtx.Done():
		t.Fatal("restored sibling tree did not finish")
	}
	var parentStarts, parentResults []string
	var restoredMetadata *runs.ToolCallFinished
	var childEnds, rootEnds, rootModels int
	for _, event := range observed {
		if event.Member.MemberID == processID.String() {
			t.Fatalf("completed sibling B was projected again: %+v", event)
		}
		switch payload := event.Payload.(type) {
		case runs.ToolCallStarted:
			if !event.Member.Child() {
				parentStarts = append(parentStarts, payload.SourceCallID)
			}
		case runs.ToolResultsCommitted:
			if !event.Member.Child() {
				for _, result := range payload.Results {
					parentResults = append(parentResults, result.ModelResult.ID)
					if result.ModelResult.ID == "ordinary_c" {
						restoredMetadata = new(result)
					}
				}
			}
		case runs.ModelCallCompleted:
			if !event.Member.Child() {
				rootModels++
			}
		case runs.SegmentEnded:
			if payload.Reason != run.OutcomeCompleted {
				t.Fatalf("restored member failed: %+v", payload)
			}
			if event.Member.Child() {
				childEnds++
			} else {
				rootEnds++
			}
		}
	}
	retained, retainedFound, err := projection.Item(t.Context(), known.ID())
	if err != nil || !retainedFound {
		t.Fatalf("retained tool missing: %v", err)
	}
	if restoredMetadata != nil || !reflect.DeepEqual(retained, known) || writes.Load() != 1 || offloads.calls.Load() != 1 {
		t.Fatalf("known tool changed or reran across restore: repeated=%+v writes=%d offloads=%d", restoredMetadata, writes.Load(), offloads.calls.Load())
	}
	expectedStarts := []string{"delegate_a"}
	expectedResults := []string{"delegate_a"}
	if !slices.Equal(parentStarts, expectedStarts) || !slices.Equal(parentResults, expectedResults) ||
		childEnds != 1 || rootEnds != 1 || modelCalls.Load() != 5 || rootModels != 1 {
		t.Fatalf("restored tree starts=%v results=%v childEnds=%d rootEnds=%d modelCalls=%d",
			parentStarts, parentResults, childEnds, rootEnds, modelCalls.Load())
	}
}

type siblingEditHook struct{}

func (siblingEditHook) BeforeToolUse(_ context.Context, input InteractionToolHookInput) (InteractionToolHookDecision, error) {
	if input.ToolName != "fixture_store" {
		return allowToolHook(false, nil), nil
	}
	arguments, err := domaintool.ParseArguments(`{"value":"edited"}`)
	if err != nil {
		return InteractionToolHookDecision{}, err
	}
	return allowToolHook(false, &arguments), nil
}

func (siblingEditHook) AfterToolUse(context.Context, InteractionToolHookInput) error { return nil }

type recordingToolResultOffloader struct {
	store toolResultOffloader
	calls atomic.Int32
}

func (s *recordingToolResultOffloader) Stage(ctx context.Context, stage transcript.ToolResultStage) error {
	if err := s.store.Stage(ctx, stage); err != nil {
		return err
	}
	s.calls.Add(1)
	return nil
}
