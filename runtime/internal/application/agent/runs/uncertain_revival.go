package runs

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	"github.com/Tangerg/flame/runtime/internal/application/ownership"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
)

// uncertainRevivalSource is the durable evidence revival reads: the open Runs,
// their Transcript, and the Tool attempts that never settled.
type uncertainRevivalSource interface {
	ListNonTerminalRuns(context.Context) ([]run.Run, error)
	ListTranscript(ctx context.Context, sessionID identity.SessionID) ([]transcript.Item, error)
	ListOpenToolInvocations(context.Context) ([]OpenToolInvocation, error)
}

type ActiveExecution struct {
	Run     run.Run
	Session session.Session
	Tools   []OpenToolInvocation
	Items   map[identity.ItemID]transcript.Item
}

type uncertainExecutionRecovery interface {
	StageUncertainExecution(context.Context, ActiveExecution) (UncertainRecovery, error)
	BeginUncertainExecution(context.Context, ExecutorRef) error
}

type UncertainRecovery interface{ uncertainRecovery() }

type RecoveryUnavailable struct{}
type RecoveryDeferred struct{}

type StagedRecovery struct {
	Ref      ExecutorRef
	MemberID string
	Tools    []RecoveredTool
}

func (RecoveryUnavailable) uncertainRecovery() {}
func (RecoveryDeferred) uncertainRecovery()    {}
func (StagedRecovery) uncertainRecovery()      {}

type RecoveredTool struct {
	Start      ToolCallStarted
	Invocation OpenToolInvocation
}

var ErrOriginalSourcePending = errors.New("runs: original source is still connecting")

// A deferred root retains its admission so the sweep can recover other Sessions
// without closing this Run as lost while its original source connects.
type UncertainRevival struct {
	deferred map[identity.RunID]ownership.RunAdmission
}

// The coordinator reinstalls the original writer lease and Segment pump before
// the executor restores Scope or publishes a verified external result.
func (r *UncertainRevival) Reconcile(ctx context.Context, c *Coordinator, source uncertainRevivalSource, executor uncertainExecutionRecovery) error {
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
		err = r.reviveRoot(ctx, c, source, executor, candidate, sess, admission)
		if err != nil {
			return fmt.Errorf("runs: revive uncertain execution %q: %w", candidate.ID(), err)
		}
	}
	if len(r.deferred) > 0 {
		return ErrOriginalSourcePending
	}
	return nil
}

func (r *UncertainRevival) reviveRoot(ctx context.Context, c *Coordinator, source uncertainRevivalSource, executor uncertainExecutionRecovery, candidate run.Run, sess session.Session, admission ownership.RunAdmission) error {
	delete(r.deferred, candidate.ID())
	defer func() {
		if _, pending := r.deferred[candidate.ID()]; !pending {
			admission.Release()
		}
	}()
	values, err := c.runs.Tree(ctx, candidate.ID())
	if err != nil {
		return err
	}
	// Receipt recovery only proves the original root and its Tool effects.
	// Delegated Runs require a separate proof of their product handoff.
	if len(values) != 1 || values[0].State() != run.Running {
		return nil
	}
	root := values[0]
	items, err := source.ListTranscript(ctx, root.SessionID())
	if err != nil {
		return err
	}
	tools, err := source.ListOpenToolInvocations(ctx)
	if err != nil {
		return err
	}
	byID := make(map[identity.ItemID]transcript.Item, len(items))
	for _, item := range items {
		byID[item.ID()] = item
	}
	active := ActiveExecution{Run: root, Session: sess, Tools: tools, Items: byID}
	decision, err := executor.StageUncertainExecution(ctx, active)
	if err != nil {
		return err
	}
	switch result := decision.(type) {
	case RecoveryUnavailable:
		return nil
	case RecoveryDeferred:
		if r.deferred == nil {
			r.deferred = make(map[identity.RunID]ownership.RunAdmission)
		}
		r.deferred[candidate.ID()] = admission
		return nil
	case StagedRecovery:
		return c.openRevivedSegment(ctx, active, admission, result, executor)
	default:
		return errors.New("runs: missing uncertain recovery outcome")
	}
}

func (c *Coordinator) openRevivedSegment(ctx context.Context, active ActiveExecution, admission ownership.RunAdmission, recovered StagedRecovery, executor uncertainExecutionRecovery) (err error) {
	staged := ownStagedExecution(c.segments.releases, recovered.Ref)
	defer func() { err = staged.abandon(ctx, err, "staged recovery") }()

	root := active.Run
	member := Continuation{RunID: root.ID(), MemberID: recovered.MemberID}
	for _, call := range recovered.Tools {
		member.DrainedTools = append(member.DrainedTools, DrainedTool{
			ItemID: call.Invocation.ItemID, CallID: call.Invocation.CallID, SourceCallID: call.Start.SourceCallID,
		})
	}
	handoff := &treeContinuation{
		rootRunID: root.ID(), runs: map[identity.RunID]run.Run{root.ID(): root}, items: active.Items,
		continuations: []Continuation{member}, recoveredTools: recovered.Tools,
	}
	_, err = c.segments.openSegment(ctx, segmentSpec{
		RunID: root.ID(), SegmentID: root.ActiveSegmentID(), SessionID: root.SessionID(),
		ExecutorID: recovered.Ref.ExecutorID, WorkspaceCWD: active.Session.Workspace().Path(), Isolated: active.Session.Isolated(),
		Opening: treeResumption{Continuation: handoff}, admission: &admission, staged: staged,
		BeginExecution: func(ctx context.Context) error { return executor.BeginUncertainExecution(ctx, recovered.Ref) },
	})
	return err
}
