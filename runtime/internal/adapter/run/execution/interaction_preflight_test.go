package execution

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	modeladapter "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/interrupt"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	modelchoice "github.com/Tangerg/flame/runtime/model"
	"github.com/Tangerg/scope/core/chat"
)

func TestContinuationPreflightDoesNotAssembleOrAcquireAWriter(t *testing.T) {
	workspace := t.TempDir()
	checkpoint := captureInteractionQuestionCheckpoint(t, workspace)
	waiting := rootInteractionExecutionContinuation(checkpoint, workspace, "exec_preflight", run.Capabilities{InterruptKinds: []interrupt.Kind{interrupt.Question}})
	state, err := decodeExecutorCheckpoint(checkpoint)
	if err != nil {
		t.Fatal(err)
	}
	var answers []runs.InterruptAnswer
	for wait, target := range state.handoff.inputs {
		answers = append(answers, runs.InterruptAnswer{MemberID: target.memberID.String(), RequestID: wait.String(), InterruptItemID: "item_answer", Resolution: testsupport.QuestionAnswer(t, [][]string{{"chosen"}})})
	}
	executor := newObservedTestInteractionExecutor(t, chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
		t.Fatal("preflight invoked a model")
		return nil, nil
	}), InteractionExecutorConfig{})
	assemblyFailure := errors.New("assembly is forbidden before acceptance")
	assemblies := 0
	executor.config.ChatResolver = interactionChatResolverFunc(func(context.Context, modelchoice.Selection) (modeladapter.ResolvedChat, error) {
		assemblies++
		return modeladapter.ResolvedChat{}, assemblyFailure
	})
	before, found, err := executor.config.ExecutionTrees.LoadExecutionTree(t.Context(), waiting.SessionID, checkpoint.RootMemberID)
	if err != nil || !found {
		t.Fatalf("head=%t,%v", found, err)
	}
	for range 2 {
		requests, err := executor.PrepareContinuation(t.Context(), waiting, answers, nil)
		if err != nil || len(requests) != 0 {
			t.Fatalf("complete preflight=%v,%v", requests, err)
		}
	}
	if _, err := executor.PrepareContinuation(t.Context(), waiting, nil, nil); !errors.Is(err, runs.ErrExecutorStateLost) {
		t.Fatalf("partial answer set=%v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := executor.PrepareContinuation(ctx, waiting, answers, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled preflight=%v", err)
	}
	failure := errors.New("head is unavailable")
	readFailure := &answerReadFailure{ExecutionTreeStore: testTrees(t), cause: failure}
	readFailure.reads.Store(1)
	executor.config.ExecutionTrees = readFailure
	if _, err := executor.PrepareContinuation(t.Context(), waiting, answers, nil); !errors.Is(err, failure) || errors.Is(err, runs.ErrExecutorStateLost) {
		t.Fatalf("inconclusive preflight lost its cause=%v", err)
	}
	executor.config.ExecutionTrees = testTrees(t)
	after, found, err := executor.config.ExecutionTrees.LoadExecutionTree(t.Context(), waiting.SessionID, checkpoint.RootMemberID)
	if err != nil || !found || !before.SameCommit(after) || assemblies != 0 {
		t.Fatalf("preflight changed the head or assembled resources: found=%t assemblies=%d err=%v", found, assemblies, err)
	}
	if _, err := executor.session(runs.ExecutorRef{SessionID: waiting.SessionID, ExecutorID: waiting.ExecutorID}); !errors.Is(err, runs.ErrExecutorNotLive) {
		t.Fatalf("preflight published an executor: %v", err)
	}
	if _, err := executor.StageAcceptedContinuation(t.Context(), waiting); !errors.Is(err, assemblyFailure) || assemblies != 1 {
		t.Fatalf("accepted restoration did not own assembly=%v, assemblies=%d", err, assemblies)
	}
}

func TestContinuationPreflightUsesProductRetirementBeforeScopeDrain(t *testing.T) {
	fixture := newWaitingDelegateFixture(t, "retired-before-drain")
	started := fixture.start(t)
	drained := make(chan struct{})
	go func() {
		for range started.Events {
		}
		close(drained)
	}()
	barrier := fixture.waitForBarrier(t, 2*time.Second)
	<-drained
	waiting := waitingDelegateContinuation(barrier, fixture.workspace)
	if len(waiting.Members) != 2 {
		t.Fatalf("waiting members=%v", waiting.Members)
	}
	ref := runs.ExecutorRef{SessionID: waiting.SessionID, ExecutorID: waiting.ExecutorID}
	defer fixture.executor.Release(t.Context(), ref)
	before, found, err := fixture.executor.config.ExecutionTrees.LoadExecutionTree(t.Context(), waiting.SessionID, waiting.Checkpoint.RootMemberID)
	if err != nil || !found {
		t.Fatalf("waiting head=%t,%v", found, err)
	}
	// The accepted product cut has retired the child while Scope still holds its
	// paused wait. Those facts need not transition in the same transaction.
	waiting.Members = slices.DeleteFunc(waiting.Members, func(member runs.ExecutionMember) bool { return member.RunID != waiting.RootRunID })
	waiting.Members[0].DrainedTools = nil
	if _, err := fixture.executor.PrepareContinuation(t.Context(), waiting, nil, nil); err != nil {
		t.Fatalf("retired product member was required to finish Scope first: %v", err)
	}
	after, found, err := fixture.executor.config.ExecutionTrees.LoadExecutionTree(t.Context(), waiting.SessionID, waiting.Checkpoint.RootMemberID)
	if err != nil || !found || !before.SameCommit(after) {
		t.Fatalf("preflight advanced the draining tree: found=%t,err=%v", found, err)
	}
}
