package runs

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
)

type refusedTreeBarrierEffects struct {
	fakeEffects
	barrierCalls atomic.Int32
}

func (f *refusedTreeBarrierEffects) CommitTreeBarrier(context.Context, TreeBarrierCommit) error {
	f.barrierCalls.Add(1)
	return errors.New("tree barrier transaction refused")
}

func TestFailedTreeBarrierRetainsCommittedToolForTerminalClosure(t *testing.T) {
	root := ExecutorMember{MemberID: "member_root"}
	executor := &fakeExecutor{executorEvents: []ExecutorEvent{
		{Member: root, Payload: ToolCallStarted{
			CallID: "call_started", SourceCallID: "source_started", ModelCallSequence: 1,
			ToolName: "shell", Arguments: `{"command":"pwd"}`, SafetyClass: tool.SafetyClassExec,
		}},
		{Member: root, Payload: mustTreeInterrupted(t, testExecutorCheckpoint(), []MemberInterruption{{
			MemberID: root.MemberID, RequestID: "request_question", Interrupt: treeBarrierQuestion("Continue?"),
		}})},
	}}
	effects := &refusedTreeBarrierEffects{}
	coordinator := testCoordinator(executor, effects)
	spec := testSegment()
	spec.Capabilities = run.Capabilities{InterruptKinds: []run.InterruptKind{run.InterruptQuestion}}
	stream, err := coordinator.openSegment(t.Context(), spec)
	if err != nil {
		t.Fatal(err)
	}
	collectEvents(stream)
	if effects.barrierCalls.Load() != 1 {
		t.Fatalf("barrier commits = %d, want one refused transaction", effects.barrierCalls.Load())
	}
	var starts, closures, invocationStarts, invocationClosures, terminal int
	for _, commit := range effects.commitSnapshot() {
		if commit.State == StateTerminalize && commit.Run != nil && commit.Run.State() == run.Failed {
			terminal++
		}
		for _, item := range commit.Items {
			if item.Kind() != transcript.ToolCall {
				continue
			}
			if item.Status() == transcript.ItemRunning {
				starts++
			} else {
				closures++
			}
		}
		for _, invocation := range commit.ToolInvocations {
			if invocation.State == ToolInvocationStarted {
				invocationStarts++
			} else {
				invocationClosures++
			}
		}
	}
	if starts != 1 || closures != 1 || invocationStarts != 1 || invocationClosures != 1 || terminal != 1 {
		t.Fatalf("committed starts/closures=%d/%d, invocation starts/closures=%d/%d, failed terminals=%d", starts, closures, invocationStarts, invocationClosures, terminal)
	}
}
