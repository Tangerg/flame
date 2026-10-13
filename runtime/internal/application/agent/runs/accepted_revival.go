package runs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"slices"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
)

// ReviveAcceptedContinuations runs before abandoned ownership is classified.
// An inconclusive read/restore stops startup and retains the handoff; only a
// proven incompatible boundary yields to the existing active-Run loss policy.
func (c *Coordinator) ReviveAcceptedContinuations(ctx context.Context) error {
	accepted, err := c.acceptedContinuations.List(ctx)
	if err != nil {
		return fmt.Errorf("runs: list accepted continuations: %w", err)
	}
	for _, value := range accepted {
		if err := c.reviveAcceptedContinuation(ctx, value); err != nil {
			return fmt.Errorf("runs: revive accepted continuation %q: %w", value.RootRunID(), err)
		}
	}
	return nil
}

func (c *Coordinator) reviveAcceptedContinuation(ctx context.Context, accepted AcceptedContinuation) error {
	if err := accepted.Validate(); err != nil {
		return err
	}
	sess, err := c.sessionReader.Get(ctx, accepted.SessionID)
	if err != nil {
		return err
	}
	admission, acquired, err := c.admission.AcquireRun(ctx, accepted.SessionID, sess.Workspace().Path())
	if err != nil || !acquired {
		return err
	}
	defer admission.Release()
	values, err := c.runs.Tree(ctx, accepted.RootRunID())
	if err != nil {
		return err
	}
	root, found := parkedRoot(accepted.RootRunID(), values)
	if !found {
		return errors.New("runs: accepted continuation root is missing")
	}
	if root.State().IsTerminal() {
		return c.acceptedContinuations.Delete(ctx, accepted.RootRunID())
	}
	if err := accepted.validateRuns(values); err != nil {
		return c.retireUnrestorableAccepted(ctx, accepted, err)
	}
	items, err := c.pendingItems(ctx, accepted.Pending)
	if err != nil {
		return err
	}
	answers, err := accepted.answers(items)
	if err != nil {
		return c.retireUnrestorableAccepted(ctx, accepted, err)
	}
	var input *CommittedUserInput
	if accepted.InputItemID != "" {
		item, found, err := c.items.Item(ctx, accepted.InputItemID)
		if err != nil {
			return err
		}
		if !found || item.Kind() != transcript.UserMessage || item.SessionID() != accepted.SessionID || item.RunID() != root.ID() {
			return c.retireUnrestorableAccepted(ctx, accepted, errors.New("runs: accepted input Item is missing or has another owner"))
		}
		input = &CommittedUserInput{ItemID: item.ID(), Content: item.Content()}
	}
	member, _ := accepted.Pending.RootContinuation()
	checkpoint, err := c.checkpoints.ReadWaitingCheckpoint(ctx, member.MemberID)
	if err != nil {
		if errors.Is(err, ErrExecutorCheckpointNotFound) || errors.Is(err, ErrInvalidExecutorCheckpoint) {
			return c.retireUnrestorableAccepted(ctx, accepted, err)
		}
		return err
	}
	waiting, err := waitingContinuationFromPending(accepted.Pending, checkpoint, values, sess)
	if err != nil {
		return c.retireUnrestorableAccepted(ctx, accepted, err)
	}
	continuation, err := accepted.treeContinuation(values, items)
	if err != nil {
		return c.retireUnrestorableAccepted(ctx, accepted, err)
	}
	ref, err := c.continuation.StageAcceptedContinuation(ctx, waiting, answers)
	if err != nil {
		if errors.Is(err, ErrExecutorStateLost) || errors.Is(err, ErrInvalidExecutorCheckpoint) {
			return c.retireUnrestorableAccepted(ctx, accepted, err)
		}
		return err
	}
	staged := ownStagedExecution(c.releases, ref)
	_, err = c.openSegment(ctx, segmentSpec{
		RunID: root.ID(), SegmentID: root.ActiveSegmentID(), SessionID: accepted.SessionID,
		WorkspaceCWD: sess.Workspace().Path(), Isolated: sess.Isolated(), ExecutorID: ref.ExecutorID,
		Continuation: continuation, AcceptedContinuation: &accepted, admission: &admission,
		BeginExecution: func(beginCtx context.Context) error {
			if err := c.continuation.BeginContinuation(beginCtx, ref, answers, input); err != nil {
				return err
			}
			return nil
		},
	})
	if err != nil {
		return staged.abandon(ctx, err, "accepted continuation")
	}
	staged.transfer()
	return nil
}

func (c *Coordinator) retireUnrestorableAccepted(ctx context.Context, accepted AcceptedContinuation, cause error) error {
	slog.WarnContext(ctx, "runs: accepted continuation is not restorable", "run.id", accepted.RootRunID(), "error", cause)
	return c.acceptedContinuations.Delete(ctx, accepted.RootRunID())
}

func (a AcceptedContinuation) validateRuns(values []run.Run) error {
	active := make(map[string]run.Run)
	for _, value := range values {
		if !value.State().IsTerminal() {
			active[value.ID()] = value
		}
	}
	if len(active) != len(a.Segments) {
		return errors.New("runs: accepted continuation member set changed")
	}
	for _, segment := range a.Segments {
		value, found := active[segment.RunID]
		if !found || value.SessionID() != a.SessionID || value.State() != run.Running || value.ActiveSegmentID() != segment.SegmentID {
			return errors.New("runs: accepted continuation segment changed")
		}
	}
	return nil
}

func (a AcceptedContinuation) treeContinuation(values []run.Run, items map[string]transcript.Item) (*treeContinuation, error) {
	continuation := &treeContinuation{rootRunID: a.RootRunID(), continuations: slices.Clone(a.Pending.Continuations), runs: parkedRunsByID(values), items: maps.Clone(items), approvalCalls: make(map[string]string)}
	for index, binding := range a.Pending.Bindings {
		item := items[binding.InterruptItemID]
		open := a.Pending.Interrupts[index]
		value := transcript.Interrupt{ItemID: item.ID(), RunID: item.RunID(), ItemOccurredAt: item.OccurredAt(), Kind: open.Kind()}
		if question, ok := item.Question(); ok {
			value.Question = &question
		} else {
			invocation, ok := item.ToolInvocation()
			if !ok || open.Approval == nil {
				return nil, errors.New("runs: accepted approval has no Tool invocation")
			}
			value.Approval = &transcript.Approval{Tool: invocation, Risk: open.Approval.Risk, Reason: open.Approval.Reason, Rememberable: open.Approval.Rememberable}
			continuation.approvalCalls[item.ID()] = binding.ToolCallID
		}
		continuation.interrupts = append(continuation.interrupts, value)
	}
	return continuation, continuation.validate()
}
