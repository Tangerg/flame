package runs

import (
	"context"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
)

type SessionExecutionContexts interface {
	LoadSessionCheckpoint(context.Context, string) (ExecutorCheckpoint, bool, error)
}

type ActiveExecution struct {
	Run     run.Run
	Session session.Session
	Context ExecutorCheckpoint
	Tools   []OpenToolInvocation
	Items   []transcript.Item
}

type UncertainExecutionRecovery interface {
	PrepareUncertainExecution(context.Context, ActiveExecution) (UncertainExecutionPlan, bool, error)
	StageUncertainExecution(context.Context, ExecutionContinuation) (ExecutorRef, error)
	BeginUncertainExecution(context.Context, ExecutorRef) error
}

type RecoveredTool struct {
	Start      ToolCallStarted
	Invocation OpenToolInvocation
}

type UncertainExecutionPlan struct {
	Continuation ExecutionContinuation
	Tools        []RecoveredTool
}

// The coordinator reinstalls the original writer lease and Segment pump before
// the executor restores Scope or publishes a verified external result.
func (c *Coordinator) ReviveUncertainExecutions(ctx context.Context, source RecoveryStore, contexts SessionExecutionContexts, executor UncertainExecutionRecovery) error {
	values, err := source.ListNonTerminalRuns(ctx)
	if err != nil {
		return err
	}
	for _, candidate := range values {
		if candidate.State() != run.Running || !candidate.Lineage().IsRoot() {
			continue
		}
		sess, err := c.sessionReader.Get(ctx, candidate.SessionID())
		if err != nil {
			return err
		}
		admission, acquired, err := c.admission.AcquireRun(ctx, sess.ID(), sess.Workspace().Path())
		if err != nil {
			return err
		}
		if !acquired {
			continue
		}
		err = func() error {
			defer admission.Release()
			values, err := c.runs.Tree(ctx, candidate.ID())
			if err != nil {
				return err
			}
			// Batch C admits one product root and its ordinary Tool children.
			// Delegated active trees need their own proven recovery class.
			if len(values) != 1 || values[0].State() != run.Running {
				return nil
			}
			root := values[0]
			checkpoint, found, err := contexts.LoadSessionCheckpoint(ctx, root.SessionID())
			if err != nil || !found {
				return err
			}
			items, err := source.ListTranscript(ctx, root.SessionID())
			if err != nil {
				return err
			}
			tools, err := source.ListOpenToolInvocations(ctx)
			if err != nil {
				return err
			}
			plan, eligible, err := executor.PrepareUncertainExecution(ctx, ActiveExecution{Run: root, Session: sess, Context: checkpoint, Tools: tools, Items: items})
			if err != nil || !eligible {
				return err
			}
			continuation := plan.Continuation
			byID := make(map[string]transcript.Item, len(items))
			for _, item := range items {
				byID[item.ID()] = item
			}
			handoff := &treeContinuation{recoveredTools: plan.Tools, rootRunID: root.ID(), runs: map[string]run.Run{root.ID(): root}, items: byID, continuations: []Continuation{{RunID: root.ID(), MemberID: checkpoint.RootMemberID, DrainedTools: continuation.Members[0].DrainedTools}}}
			ref := ExecutorRef{SessionID: continuation.SessionID, ExecutorID: continuation.ExecutorID}
			_, err = c.openSegment(ctx, segmentSpec{RunID: root.ID(), SegmentID: root.ActiveSegmentID(), SessionID: root.SessionID(), WorkspaceCWD: sess.Workspace().Path(), Isolated: sess.Isolated(), ExecutorID: ref.ExecutorID, Continuation: handoff, admission: &admission,
				StageExecution: func(ctx context.Context) error {
					_, err := executor.StageUncertainExecution(ctx, continuation)
					return err
				},
				BeginExecution: func(ctx context.Context) error { return executor.BeginUncertainExecution(ctx, ref) },
			})
			return err
		}()
		if err != nil {
			return fmt.Errorf("runs: revive uncertain execution %q: %w", candidate.ID(), err)
		}
	}
	return nil
}
