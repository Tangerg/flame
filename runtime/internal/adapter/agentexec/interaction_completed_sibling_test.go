package agentexec

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/application/ownership"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/interrupt"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	agent "github.com/Tangerg/scope/agent"
	"github.com/Tangerg/scope/agent/interaction"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

func TestInteractionExecutorRestoresWaitingTreeWithCompletedSibling(t *testing.T) {
	bStarted := make(chan agent.ProcessID, 1)
	releaseA := make(chan struct{})
	var modelCalls atomic.Int32
	model := chat.ModelFunc(func(ctx context.Context, request *chat.Request) (*chat.Response, error) {
		modelCalls.Add(1)
		switch {
		case hasToolMessage(request.Messages):
			return interactionUsageTextResponse("continued", 2, 1), nil
		case userMessagesContain(request.Messages, "waiting sibling A"):
			select {
			case <-releaseA:
				return interactionToolResponse(chat.ToolCall{ID: "ask_a", Name: "ask", Arguments: `{}`}, 2, 1), nil
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
			return interactionToolBatchResponse([]chat.ToolCall{
				{ID: "delegate_a", Name: "delegate_task", Arguments: `{"summary":"A","instructions":"waiting sibling A"}`},
				{ID: "delegate_b", Name: "delegate_task", Arguments: `{"summary":"B","instructions":"completed sibling B"}`},
			}, 2, 1), nil
		}
	})
	client, err := chatclient.New(model, chatclient.Config{})
	if err != nil {
		t.Fatal(err)
	}
	question, err := toolcontract.NewFunc(toolcontract.FuncConfig{Name: "ask", Description: "Ask for a value."}, waitingDelegateQuestion)
	if err != nil {
		t.Fatal(err)
	}
	executor, err := NewInteractionExecutor(InteractionExecutorConfig{
		Lifetime: t.Context(), ChatResolver: staticInteractionChatResolver(client),
		ImplementationIdentity: "completed-sibling-build", ConfigurationIdentity: "completed-sibling-config",
		DefaultMaxModelCalls: uint32Pointer(6), MaxConcurrentToolCalls: intPointer(4), BuildID: interactionTestBuildID,
		ToolResolver:    staticInteractionTools{manifest: toolset.Manifest{Visible: []toolcontract.Tool{question}}},
		ToolInterpreter: testInteractionToolInterpreter{}, ToolAuthorizer: allowInteractionTools{},
	})
	if err != nil {
		t.Fatal(err)
	}
	sessions := &delegateSessionStore{value: testsupport.MustRestoreSession(session.Snapshot{
		ID: "session_1", Title: "completed sibling", Workspace: testsupport.MustWorkspace(t.TempDir()),
	})}
	projection := newDelegateProjection()
	runSequence, segmentSequence := 0, 0
	coordinator := mustNewRunCoordinator(t, runs.Dependencies{
		RootStarts: executor, Observations: executor, Releases: executor, Conversation: delegateConversation{},
		Session: runs.SessionPorts{Reader: sessions, Creator: sessions, ActiveRuns: sessions},
		Projection: runs.ProjectionPorts{
			Openings: projection, ChildStarts: projection, Events: projection, Barriers: projection,
			Checkpoints: projection, Workspace: projection, Finalizer: projection,
		},
		Admissions: new(ownership.Gate), Now: time.Now,
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
		SessionID: "session_1", Capabilities: run.Capabilities{ChildRuns: true, InterruptKinds: []interrupt.Kind{interrupt.Question}},
		Input: []transcript.ContentBlock{{Kind: transcript.TextContent, Text: "delegate both siblings"}},
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
	close(releaseA)
	fixture := &waitingDelegateFixture{executor: executor, coordinator: coordinator, projection: projection}
	barrier := fixture.waitForBarrier(t, 2*time.Second)
	<-eventsReady
	pending := barrier.Pending()
	if len(pending.Continuations) != 2 || len(pending.Bindings) != 1 {
		t.Fatalf("waiting tree includes a completed sibling: %+v", pending)
	}
	for _, member := range pending.Continuations {
		if member.MemberID == processID.String() {
			t.Fatal("completed sibling B is a waiting continuation member")
		}
	}
	if err := executor.Release(t.Context(), runs.ExecutorRef{SessionID: pending.SessionID, ExecutorID: pending.ExecutorID}); err != nil {
		t.Fatal(err)
	}
	ref, err := executor.StageContinuation(t.Context(), waitingDelegateContinuation(barrier))
	if err != nil {
		t.Fatalf("restore completed sibling beside a waiting member: %v", err)
	}
	sequence, err := executor.Observe(t.Context(), ref)
	if err != nil {
		t.Fatal(err)
	}
	continuedEvents := collectInteractionEvents(sequence)
	binding := pending.Bindings[0]
	if err := executor.BeginContinuation(t.Context(), ref, []runs.InterruptAnswer{{
		InterruptItemID: binding.InterruptItemID, MemberID: binding.MemberID,
		RequestID: binding.RequestID, Resolution: interrupt.Resolution{Answers: [][]string{{"chosen"}}},
	}}, nil, pending.Capabilities.InterruptKinds); err != nil {
		t.Fatal(err)
	}
	var observed []runs.ExecutorEvent
	select {
	case observed = <-continuedEvents:
	case <-ctx.Done():
		t.Fatal("restored sibling tree did not finish")
	}
	var parentStarts, parentResults []string
	var childEnds, rootEnds int
	for _, event := range observed {
		if event.Member.MemberID == processID.String() {
			t.Fatalf("completed sibling B was projected again: %+v", event)
		}
		switch payload := event.Payload.(type) {
		case runs.ToolCallStarted:
			if !event.Member.Child() {
				parentStarts = append(parentStarts, payload.SourceCallID)
			}
		case runs.ToolCallFinished:
			if !event.Member.Child() && payload.ModelResult != nil {
				parentResults = append(parentResults, payload.ModelResult.ID)
			}
		case runs.SegmentEnded:
			if payload.Reason != run.OutcomeCompleted {
				t.Fatalf("restored member failed: %+v", payload)
			}
			if event.Member.Child() {
				childEnds++
			} else {
				rootEnds++
				if payload.Usage() == nil || payload.Usage().Steps != 2 {
					t.Fatalf("restored root accounting = %+v, want two root model calls", payload.Usage())
				}
			}
		}
	}
	wantCalls := []string{"delegate_a", "delegate_b"}
	if !slices.Equal(parentStarts, wantCalls) || !slices.Equal(parentResults, wantCalls) ||
		childEnds != 1 || rootEnds != 1 || modelCalls.Load() != 5 {
		t.Fatalf("restored tree starts=%v results=%v childEnds=%d rootEnds=%d modelCalls=%d",
			parentStarts, parentResults, childEnds, rootEnds, modelCalls.Load())
	}
	if err := executor.Release(t.Context(), ref); err != nil {
		t.Fatal(err)
	}
}
