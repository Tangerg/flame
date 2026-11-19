package runs

import (
	"context"
	"errors"
	"fmt"

	rundomain "github.com/Tangerg/flame/runtime/internal/domain/run"
)

// ReviveAcceptedSubtreeCancellations runs before ordinary Run-loss recovery.
// The reduced product tree cannot prove that Scope has retired its old members.
func (c *Cancellations) ReviveAcceptedSubtreeCancellations(ctx context.Context) error {
	accepted, err := c.acceptedSubtreeCancellations.SubtreeCancellations(ctx, "")
	if err != nil {
		return err
	}
	for _, value := range accepted {
		if _, live := c.segments.registry.Running(value.RootRunID()); live {
			continue
		}
		if err := c.reviveAcceptedSubtreeCancellation(ctx, value); err != nil && !errors.Is(err, rundomain.ErrSessionBusy) {
			return fmt.Errorf("runs: revive accepted cancellation of %q: %w", value.TargetRunID, err)
		}
	}
	return nil
}

func (c *Cancellations) SettleAcceptedSubtreeCancellation(ctx context.Context, ref ExecutorRef, rootRunID, targetRunID string) error {
	cleanupCtx, cancel := runCleanupContext(ctx)
	defer cancel()
	values, err := c.acceptedSubtreeCancellations.SubtreeCancellations(cleanupCtx, rootRunID)
	if err != nil {
		return err
	}
	if len(values) == 0 || values[0].TargetRunID != targetRunID {
		return nil
	}
	accepted := values[0]
	if accepted.SessionID != ref.SessionID || accepted.Pending.ExecutorID != ref.ExecutorID {
		return ErrInvalidExecutorRef
	}
	if live, ok := c.segments.registry.Running(rootRunID); ok {
		if live.record.ExecutorID != ref.ExecutorID || len(accepted.Segments) == 0 ||
			live.record.SegmentID != accepted.Segments[len(accepted.Segments)-1].SegmentID {
			return ErrInvalidExecutorRef
		}
		if err := live.owner.awaitActivation(cleanupCtx); err != nil {
			return &CancellationSettlementError{Cause: err}
		}
		if err := c.acceptedSubtreeCancellations.DeleteSubtreeCancellation(cleanupCtx, rootRunID, targetRunID); err != nil {
			return &CancellationSettlementError{Cause: err}
		}
		return nil
	}
	if err := c.reviveAcceptedSubtreeCancellation(cleanupCtx, accepted); err != nil {
		return &CancellationSettlementError{Cause: err}
	}
	return nil
}

func (c *Cancellations) reviveAcceptedSubtreeCancellation(ctx context.Context, accepted AcceptedSubtreeCancellation) (err error) {
	sess, err := c.sessionReader.Get(ctx, accepted.SessionID)
	if err != nil {
		return err
	}
	admission, acquired, err := c.admission.AcquireRun(ctx, sess.ID(), sess.Workspace().Path())
	if err != nil {
		return err
	}
	if !acquired {
		return rundomain.ErrSessionBusy
	}
	defer admission.Release()
	retained, err := c.acceptedSubtreeCancellations.SubtreeCancellations(ctx, accepted.RootRunID())
	if err != nil {
		return err
	}
	if len(retained) == 0 || retained[0].TargetRunID != accepted.TargetRunID {
		return nil
	}
	accepted = retained[0]
	values, err := c.runs.Tree(ctx, accepted.RootRunID())
	if err != nil {
		return err
	}
	root, found := parkedRoot(accepted.RootRunID(), values)
	if !found {
		return errors.New("runs: accepted cancellation root is missing")
	}
	if root.State().IsTerminal() {
		ref := ExecutorRef{SessionID: accepted.SessionID, ExecutorID: accepted.Pending.ExecutorID}
		if err := c.SettleRootCancellation(ctx, ref, root.ID()); err != nil {
			return err
		}
		return c.acceptedSubtreeCancellations.DeleteSubtreeCancellation(ctx, accepted.RootRunID(), accepted.TargetRunID)
	}
	target, found := runByID(values, accepted.TargetRunID)
	if !found || !target.Canceled() || target.Lineage().RootRunID() != root.ID() {
		return errors.New("runs: accepted cancellation target is missing its terminal decision")
	}
	ending, _ := target.Ending()
	canceled := ending.(rundomain.Canceled)
	var targetMemberID string
	for _, member := range accepted.Pending.Continuations {
		if member.RunID == target.ID() {
			targetMemberID = member.MemberID
			break
		}
	}
	rootMember, _ := accepted.Pending.RootContinuation()
	checkpoint, err := c.checkpoints.ReadWaitingCheckpoint(ctx, rootMember.MemberID)
	if err != nil {
		return err
	}
	continuation, err := waitingContinuationFromPending(accepted.Pending, checkpoint, values, sess)
	if err != nil {
		return err
	}
	request, err := NewWaitingSubtreeCancellationRequest(continuation, targetMemberID, canceled.Detail)
	if err != nil {
		return err
	}
	ref := ExecutorRef{SessionID: continuation.SessionID, ExecutorID: continuation.ExecutorID}
	if err := c.releases.Release(ctx, ref); err != nil && !errors.Is(err, ErrExecutorNotLive) {
		return fmt.Errorf("runs: retire original executor before accepted cancellation revival: %w", err)
	}

	if len(accepted.Segments) == 0 {
		pending, found, err := c.interrupts.LookupOpenInterrupt(ctx, root.ID())
		if err != nil {
			return err
		}
		if !found {
			return errors.New("runs: accepted waiting cancellation lost its surviving boundary")
		}
		if err := validatePendingRunTree(pending, values); err != nil {
			return err
		}
		recovered, err := c.subtreeCancellationRestorer.StageAcceptedSubtreeCancellation(ctx, request, WaitingSubtreeStaysWaiting)
		if err != nil {
			return err
		}
		if err := recovered.Settle(ctx); err != nil {
			return err
		}
		return c.acceptedSubtreeCancellations.DeleteSubtreeCancellation(ctx, root.ID(), target.ID())
	}
	if err := validateAcceptedSegments(sess.ID(), accepted.Segments, values); err != nil {
		return err
	}
	items, err := pendingItems(ctx, c.items, accepted.Pending)
	if err != nil {
		return err
	}
	handoff := &treeContinuation{rootRunID: root.ID(), runs: parkedRunsByID(values), items: items}
	for _, member := range accepted.Pending.Continuations {
		if value, found := handoff.run(member.RunID); found && value.State() == rundomain.Running {
			handoff.continuations = append(handoff.continuations, member)
		}
	}
	if err := handoff.validate(); err != nil {
		return err
	}
	recovered, err := c.subtreeCancellationRestorer.StageAcceptedSubtreeCancellation(ctx, request, WaitingSubtreeResumesRunning)
	if errors.Is(err, ErrSubtreeCancellationReleased) {
		return c.acceptedSubtreeCancellations.DeleteSubtreeCancellation(ctx, root.ID(), target.ID())
	}
	if err != nil {
		return err
	}
	staged := ownStagedExecution(c.releases, ref)
	defer func() { err = staged.abandon(ctx, err, "staged subtree cancellation") }()
	_, err = c.segments.openSegment(ctx, segmentSpec{
		RunID: root.ID(), SegmentID: root.ActiveSegmentID(), SessionID: sess.ID(),
		WorkspaceCWD: sess.Workspace().Path(), Isolated: sess.Isolated(), ExecutorID: continuation.ExecutorID,
		Continuation: handoff, admission: &admission, staged: staged,
		BeginExecution: recovered.Settle,
	})
	if err != nil {
		return err
	}
	return c.acceptedSubtreeCancellations.DeleteSubtreeCancellation(ctx, root.ID(), target.ID())
}
