package execution

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/interrupt"
	domaintool "github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	agent "github.com/Tangerg/scope/agent"
	"github.com/Tangerg/scope/agent/strategy/interaction"
	"github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

func TestAcceptedAnswerDeliveryAdvancesOnlyThePausedHead(t *testing.T) {
	for _, delivered := range []int{1, 2} {
		t.Run(fmt.Sprintf("answers_%d", delivered), func(t *testing.T) {
			question, err := toolcontract.NewFunc(toolcontract.FuncConfig{Name: "ask", Description: "Ask for a value."}, waitingDelegateQuestion)
			if err != nil {
				t.Fatal(err)
			}
			model := &observationScriptModel{responses: []*chat.Response{
				interactionToolBatchResponse([]chat.ToolCall{
					{ID: "a", Name: "ask", Arguments: `{}`},
					{ID: "b", Name: "ask", Arguments: `{}`},
				}, 2, 1), interactionUsageTextResponse("done", 3, 1),
			}}
			executor := newObservedTestInteractionExecutor(t, model, InteractionExecutorConfig{
				MaxConcurrentToolCalls: new(2),
				ToolResolver:           staticInteractionTools{identities: []domaintool.Ref{testsupport.A2ATool(t, "ask")}, manifest: toolset.Manifest{Visible: []toolcontract.Tool{concurrentInteractionTool{Tool: question}}}},
				ToolInterpreter:        immutableToolInterpreter{}, ToolAuthorizer: allowInteractionTools{},
			})
			start := interactionTestStart()
			start.CWD, start.WorkspaceCWD = t.TempDir(), ""
			start.WorkspaceCWD = start.CWD
			start.InterruptKinds = []interrupt.Kind{interrupt.Question}
			ref, err := executor.StageRoot(t.Context(), start)
			if err != nil {
				t.Fatal(err)
			}
			barrier := captureAcceptedTestBarrier(t, executor, ref, 2)
			waiting := rootInteractionWaitingContinuation(barrier.Checkpoint(), start.CWD, ref.ExecutorID, run.Capabilities{InterruptKinds: start.InterruptKinds})
			if _, err := executor.StageAcceptedContinuation(t.Context(), waiting); err != nil {
				t.Fatal(err)
			}
			session, err := executor.session(ref)
			if err != nil {
				t.Fatal(err)
			}
			var answers []runs.InterruptAnswer
			for index, binding := range barrier.Interruptions() {
				answers = append(answers, runs.InterruptAnswer{InterruptItemID: fmt.Sprintf("item_answer_%d", index), MemberID: binding.MemberID, RequestID: binding.RequestID, Resolution: interrupt.Resolution{Answers: [][]string{{"chosen"}}}})
			}
			prepared, err := session.prepareContinuationAnswers(t.Context(), answers)
			if err != nil {
				t.Fatal(err)
			}
			before, err := session.committedTree(t.Context(), session.state.processHandle().Relation().ProcessID())
			if err != nil {
				t.Fatal(err)
			}
			if err := session.deliverContinuationAnswers(t.Context(), prepared[:delivered], nil); err != nil {
				t.Fatal(err)
			}
			head, err := session.committedTree(t.Context(), before.RootID())
			if err != nil {
				t.Fatal(err)
			}
			pending, err := interaction.PendingToolInputs(head)
			if err != nil {
				t.Fatal(err)
			}
			if len(pending) != 2-delivered || head.Digest() == before.Digest() {
				t.Fatalf("delivery did not commit the new head: pending=%d", len(pending))
			}
			original := make(map[agent.ProcessID]agent.ProcessSnapshot)
			for _, snapshot := range before.ProcessSnapshots() {
				original[snapshot.Relation().ProcessID()] = snapshot
			}
			for _, snapshot := range head.ProcessSnapshots() {
				prior := original[snapshot.Relation().ProcessID()]
				if !snapshot.Status().Terminal() && (snapshot.Status() != agent.StatusPaused || snapshot.Usage().CommittedSteps != prior.Usage().CommittedSteps || snapshot.Usage().PreparedEffects != prior.Usage().PreparedEffects) {
					t.Fatalf("answer advanced execution: %s", snapshot.Status())
				}
			}
			if err := executor.Release(t.Context(), ref); err != nil {
				t.Fatal(err)
			}
			wire, err := decodeInteractionCheckpointWire(waiting.Checkpoint.Payload)
			if err != nil {
				t.Fatal(err)
			}
			wire.Handoff.Processes[0].CommittedSteps++
			advanced := waiting.Clone()
			advanced.Checkpoint.Payload, err = json.Marshal(wire)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := executor.StageAcceptedContinuation(t.Context(), advanced); !errors.Is(err, runs.ErrExecutorStateLost) {
				t.Fatalf("changed progress was restorable: %v", err)
			}
			unavailable := errors.New("head read unavailable")
			executor.config.ExecutionTrees = &answerReadFailure{ExecutionTreeStore: testTrees(t), cause: unavailable}
			if _, err := executor.StageAcceptedContinuation(t.Context(), waiting); !errors.Is(err, unavailable) || errors.Is(err, runs.ErrExecutorStateLost) {
				t.Fatalf("inconclusive read became state loss: %v", err)
			}
			executor.config.ExecutionTrees = testTrees(t)
			conflicting := append([]runs.InterruptAnswer(nil), answers...)
			for index := range conflicting {
				conflicting[index].Resolution.Answers = [][]string{{"different"}}
			}
			if _, err := executor.PrepareContinuation(t.Context(), waiting, conflicting, nil); !errors.Is(err, runs.ErrExecutorStateLost) {
				t.Fatalf("changed accepted payload passed preflight: %v", err)
			}
			// The original context survives, but restoration must use the answer head.
			if _, err := executor.StageAcceptedContinuation(context.Background(), waiting); err != nil {
				t.Fatal(err)
			}
			restored, err := executor.session(ref)
			if err != nil {
				t.Fatal(err)
			}
			remaining, err := restored.prepareContinuationAnswers(t.Context(), answers)
			if err != nil {
				t.Fatal(err)
			}
			admitted := 0
			for _, answer := range remaining {
				if answer.admitted {
					admitted++
				}
			}
			if admitted != delivered {
				t.Fatalf("restored receipts=%d, want %d", admitted, delivered)
			}
			sequence, err := observeTestInteraction(t, executor, t.Context(), ref)
			if err != nil {
				t.Fatal(err)
			}
			events := collectInteractionEvents(sequence)
			if err := executor.BeginContinuation(t.Context(), ref, answers, nil); err != nil {
				t.Fatal(err)
			}
			completed := <-events
			if results := payloadsOf[runs.ToolResultsCommitted](completed); len(results) != 2 {
				t.Fatalf("Tool results=%+v", results)
			}

		})
	}
}

type answerReadFailure struct {
	runs.ExecutionTreeStore
	reads atomic.Int32
	cause error
}

func (s *answerReadFailure) LoadExecutionTree(ctx context.Context, sessionID, rootID string) (runs.ExecutionTreeHead, bool, error) {
	if s.reads.Add(1) == 2 {
		return runs.ExecutionTreeHead{}, false, s.cause
	}
	return s.ExecutionTreeStore.LoadExecutionTree(ctx, sessionID, rootID)
}

// Hold only the host's polling worker so both concurrent Tool waits commit
// before asking the production barrier to pause them. Scope keeps executing.
func captureAcceptedTestBarrier(t *testing.T, executor *InteractionExecutor, ref runs.ExecutorRef, count int) runs.TreeInterrupted {
	t.Helper()
	session, err := executor.session(ref)
	if err != nil {
		t.Fatal(err)
	}
	session.lifetime.stopReconciling()
	sequence, err := observeTestInteraction(t, executor, t.Context(), ref)
	if err != nil {
		t.Fatal(err)
	}
	ready := make(chan runs.TreeInterrupted, 1)
	go func() {
		var result runs.TreeInterrupted
		for event := range sequence {
			if commit, ok := event.Payload.(runs.ExecutionFactCommit); ok {
				commit.Complete(nil)
			}
			if barrier, ok := event.Payload.(runs.TreeInterrupted); ok {
				result = barrier
				break
			}
		}
		ready <- result
	}()
	if err := executor.BeginRoot(t.Context(), ref); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		root := session.state.processHandle()
		if root != nil {
			head, err := session.committedTree(ctx, root.Relation().ProcessID())
			if err == nil {
				inputs, err := interaction.PendingToolInputs(head)
				if err != nil {
					t.Fatal(err)
				}
				if len(inputs) == count {
					break
				}
			}
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			t.Fatal("Tool waits did not commit")
		}
	}
	tree, interruptions, found, err := session.captureHumanInputBarrier(ctx)
	if err != nil || !found || len(interruptions) != count {
		t.Fatalf("paused barrier=%d, %t, %v", len(interruptions), found, err)
	}
	checkpoint, err := session.executorCheckpoint(tree)
	if err != nil {
		t.Fatal(err)
	}
	barrier, err := runs.NewTreeInterrupted(checkpoint, interruptions)
	if err != nil {
		t.Fatal(err)
	}
	session.state.mu.Lock()
	session.state.boundary = interactionBoundaryWaiting
	session.state.waitingCheckpoint = checkpoint.Clone()
	session.state.dispatchReady = make(chan struct{})
	session.state.mu.Unlock()
	session.lifetime.send(runs.ExecutorEvent{Member: session.executorMember(session.state.processHandle().Relation()), Payload: barrier})
	select {
	case result := <-ready:
		return result
	case <-ctx.Done():
		t.Fatal("barrier observer did not detach")
	}
	return runs.TreeInterrupted{}
}
