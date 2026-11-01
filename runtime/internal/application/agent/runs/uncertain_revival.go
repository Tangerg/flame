package runs

import (
	"context"
	"errors"
	"fmt"
	"github.com/Tangerg/flame/runtime/internal/application/ownership"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
)

// uncertainRevivalSource is the durable evidence revival reads: the open Runs,
// their Transcript, and the Tool attempts that never settled.
type uncertainRevivalSource interface {
	ListNonTerminalRuns(context.Context) ([]run.Run, error)
	ListTranscript(ctx context.Context, sessionID string) ([]transcript.Item, error)
	ListOpenToolInvocations(context.Context) ([]OpenToolInvocation, error)
}

type sessionExecutionContexts interface {
	LoadSessionCheckpoint(context.Context, string) (ExecutorCheckpoint, bool, error)
}

type ActiveExecution struct {
	Run     run.Run
	Session session.Session
	Context ExecutorCheckpoint
	Tools   []OpenToolInvocation
	Items   []transcript.Item
}

type uncertainExecutionRecovery interface {
	PrepareUncertainExecution(context.Context, ActiveExecution) (UncertainExecutionPlan, RecoveryVerdict, error)
	StageUncertainExecution(context.Context, ExecutionContinuation) (ExecutorRef, error)
	BeginUncertainExecution(context.Context, ExecutorRef) error
}

type RecoveryVerdict uint8

const (
	RecoveryUnavailable RecoveryVerdict = iota
	RecoveryDeferred
	RecoveryReady
)

var ErrOriginalSourcePending = errors.New("runs: original source is still connecting")

// Deferred roots retain their existing admission while the original source
// connects. A sweep can recover other Sessions without closing this one as lost.
type UncertainRevival struct {
	deferred map[string]ownership.RunAdmission
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
func (r *UncertainRevival) Reconcile(ctx context.Context, c *Coordinator, source uncertainRevivalSource, contexts sessionExecutionContexts, executor uncertainExecutionRecovery) error {
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
		admission, held := r.deferred[candidate.ID()]
		if !held {
			var acquired bool
			admission, acquired, err = c.admission.AcquireRun(ctx, sess.ID(), sess.Workspace().Path())
			if err != nil {
				return err
			}
			if !acquired {
				continue
			}
		}
		err = r.reviveRoot(ctx, c, source, contexts, executor, candidate, sess, admission)
		if err != nil {
			return fmt.Errorf("runs: revive uncertain execution %q: %w", candidate.ID(), err)
		}
	}
	if len(r.deferred) > 0 {
		return ErrOriginalSourcePending
	}
	return nil
}

func (r *UncertainRevival) reviveRoot(ctx context.Context, c *Coordinator, source uncertainRevivalSource, contexts sessionExecutionContexts, executor uncertainExecutionRecovery, candidate run.Run, sess session.Session, admission ownership.RunAdmission) error {

	defer func() {
		if _, pending := r.deferred[candidate.ID()]; !pending {
			admission.Release()
		}
	}()
	values, err := c.runs.Tree(ctx, candidate.ID())
	if err != nil {
		return err
	}
	// Batch C admits one product root and its ordinary Tool children.
	// Delegated active trees need their own proven recovery class.
	if len(values) != 1 || values[0].State() != run.Running {
		delete(r.deferred, candidate.ID())
		return nil
	}
	root := values[0]
	checkpoint, found, err := contexts.LoadSessionCheckpoint(ctx, root.SessionID())
	if err != nil {
		return err
	}
	if !found {
		delete(r.deferred, candidate.ID())
		return nil
	}
	items, err := source.ListTranscript(ctx, root.SessionID())
	if err != nil {
		return err
	}
	tools, err := source.ListOpenToolInvocations(ctx)
	if err != nil {
		return err
	}
	plan, verdict, err := executor.PrepareUncertainExecution(ctx, ActiveExecution{Run: root, Session: sess, Context: checkpoint, Tools: tools, Items: items})
	if err != nil {
		return err
	}
	if verdict == RecoveryDeferred {
		if r.deferred == nil {
			r.deferred = make(map[string]ownership.RunAdmission)
		}
		r.deferred[candidate.ID()] = admission
		return nil
	}
	delete(r.deferred, candidate.ID())
	if verdict != RecoveryReady {
		return nil
	}
	continuation := plan.Continuation
	byID := make(map[string]transcript.Item, len(items))
	for _, item := range items {
		byID[item.ID()] = item
	}
	handoff := &treeContinuation{recoveredTools: plan.Tools, rootRunID: root.ID(), runs: map[string]run.Run{root.ID(): root}, items: byID, continuations: []Continuation{{RunID: root.ID(), MemberID: checkpoint.RootMemberID, DrainedTools: continuation.Members[0].DrainedTools}}}
	ref := ExecutorRef{SessionID: continuation.SessionID, ExecutorID: continuation.ExecutorID}
	_, err = c.segments.openSegment(ctx, segmentSpec{RunID: root.ID(), SegmentID: root.ActiveSegmentID(), SessionID: root.SessionID(), WorkspaceCWD: sess.Workspace().Path(), Isolated: sess.Isolated(), ExecutorID: ref.ExecutorID, Continuation: handoff, admission: &admission,
		StageExecution: func(ctx context.Context) error {
			_, err := executor.StageUncertainExecution(ctx, continuation)
			return err
		},
		BeginExecution: func(ctx context.Context) error { return executor.BeginUncertainExecution(ctx, ref) },
	})
	return err
}
