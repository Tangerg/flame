package runs

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/runtime/internal/application/ownership"
	rundomain "github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
)

func (c *Cancellations) cancelWaitingChild(
	ctx context.Context,
	cmd CancelCommand,
	sessionID identity.SessionID,
) (result CancelResult, err error) {
	sess, err := c.sessionReader.Get(ctx, sessionID)
	if err != nil {
		return CancelResult{}, err
	}
	runAdmission, ok, leaseErr := c.admission.AcquireRun(ctx, sess.ID(), sess.Workspace().Path())
	if leaseErr != nil {
		return CancelResult{}, leaseErr
	}
	if !ok {
		return CancelResult{}, fmt.Errorf(
			"%w: session %q or working tree %q has a run or mutation in flight",
			rundomain.ErrSessionBusy,
			sess.ID(),
			sess.Workspace().Path(),
		)
	}
	defer runAdmission.Release()

	cleanupCtx, cancelCleanup := runCleanupContext(ctx)
	defer cancelCleanup()

	plan, err := c.planWaitingChildCancellation(cleanupCtx, cmd.RunID)
	if err != nil {
		return CancelResult{}, err
	}

	continuation, err := c.waitingChildCancellationContinuation(cleanupCtx, sess, plan)
	if err != nil {
		return CancelResult{}, err
	}
	target, _ := plan.pending.continuationForRun(cmd.RunID)
	request, err := newWaitingSubtreeCancellationRequest(
		continuation,
		target.MemberID,
		cmd.Reason,
	)
	if err != nil {
		return CancelResult{}, fmt.Errorf("runs: prepare waiting child cancellation request: %w", err)
	}
	prepared, err := c.waitingSubtreeCancellationPreparer.PrepareWaitingSubtreeCancellation(
		cleanupCtx,
		request,
	)
	if err != nil {
		return CancelResult{}, err
	}
	defer func() {
		if discardErr := prepared.Discard(); discardErr != nil {
			result = CancelResult{}
			err = errors.Join(
				err,
				fmt.Errorf("runs: discard waiting subtree change: %w", discardErr),
			)
		}
	}()

	transformation, err := prepareWaitingCancellationTransformation(
		plan,
		cmd.Reason,
		c.segments.publications.nowUTC(),
		prepared,
	)
	if err != nil {
		return CancelResult{}, err
	}
	if transformation.remaining != nil {
		return c.commitWaitingChildCancellation(cleanupCtx, plan, transformation, prepared, cmd.Receipt)
	}
	return c.resumeAfterWaitingChildCancellation(
		cleanupCtx,
		sess,
		plan,
		transformation,
		prepared,
		&runAdmission,
		cmd.Receipt,
	)
}

func (c *Cancellations) waitingChildCancellationContinuation(
	ctx context.Context,
	sess session.Session,
	plan waitingCancellationPlan,
) (ExecutionContinuation, error) {
	rootContinuation, ok := plan.pending.RootContinuation()
	if !ok {
		return ExecutionContinuation{}, errors.New("runs: waiting subtree Pending has no root continuation")
	}
	checkpoint, err := c.checkpoints.ReadWaitingCheckpoint(ctx, rootContinuation.MemberID)
	if err != nil {
		if !errors.Is(err, ErrExecutorStateLost) && !errors.Is(err, ErrExecutorCheckpointNotFound) {
			return ExecutionContinuation{}, err
		}
		lostErr := c.terminations.ApplyRunLost(
			ctx,
			plan.root.SessionID(),
			plan.root.ID(),
			c.segments.publications.nowUTC(),
		)
		if lostErr != nil {
			return ExecutionContinuation{}, errors.Join(
				err,
				fmt.Errorf("runs: recover lost waiting tree %q: %w", plan.root.ID(), lostErr),
			)
		}
		return ExecutionContinuation{}, fmt.Errorf("%w: %w", ErrRunNotFound, err)
	}
	return waitingContinuationFromPending(plan.pending, checkpoint, plan.tree, sess)
}

func (c *Cancellations) commitWaitingChildCancellation(
	ctx context.Context,
	plan waitingCancellationPlan,
	transformation waitingCancellationTransformation,
	change waitingSubtreeChange,
	receipt CancellationReceipt,
) (CancelResult, error) {
	durable, err := transformation.durableCommit(plan.pending, newRunCommitID())
	if err != nil {
		return CancelResult{}, err
	}
	result, err := c.waitingSubtreeCancellations.CommitWaitingSubtreeCancellation(
		ctx,
		durable,
		receipt,
	)
	if err != nil {
		return CancelResult{}, err
	}
	if err := change.Apply(WaitingSubtreeStaysWaiting); err != nil {
		return CancelResult{}, &CancellationSettlementError{Cause: err}
	}
	if err := c.acceptedSubtreeCancellations.DeleteSubtreeCancellation(ctx, plan.root.ID(), plan.target.ID()); err != nil {
		return CancelResult{}, &CancellationSettlementError{Cause: err}
	}
	c.publishWaitingChildCancellation(plan, transformation)
	return CancelResult{Run: result.TargetRun, RootRun: &result.RootRun}, nil
}

func (c *Cancellations) resumeAfterWaitingChildCancellation(
	ctx context.Context,
	sess session.Session,
	plan waitingCancellationPlan,
	transformation waitingCancellationTransformation,
	change waitingSubtreeChange,
	runAdmission *ownership.RunAdmission,
	receipt CancellationReceipt,
) (CancelResult, error) {
	segmentID := c.segments.newSegmentID()
	var committed WaitingSubtreeCancellationResult
	_, err := c.segments.openSegment(ctx, segmentSpec{
		RunID:        plan.root.ID(),
		SegmentID:    segmentID,
		SessionID:    plan.root.SessionID(),
		WorkspaceCWD: sess.Workspace().Path(),
		Isolated:     sess.Isolated(),
		ExecutorID:   plan.pending.ExecutorID,
		admission:    runAdmission,
		Opening: treeResumption{Continuation: transformation.continuation, CommitOpening: func(commitCtx context.Context, opening OpeningCommit) error {
			_, admitting := opening.Admission()
			resume, resuming := opening.Resume()
			if admitting || !resuming {
				return errors.New("runs: waiting child continuation produced an invalid opening disposition")
			}
			durable, durableErr := transformation.resumedDurableCommit(
				plan.pending, opening.CommitID(), resume, opening.Events(),
			)
			if durableErr != nil {
				return durableErr
			}
			result, commitErr := c.waitingSubtreeCancellations.CommitWaitingSubtreeCancellation(commitCtx, durable, receipt)
			if commitErr == nil {
				committed = result
			}
			return commitErr
		}},
		BeginExecution: func(beginCtx context.Context) error {
			if err := change.Apply(WaitingSubtreeResumesRunning); err != nil {
				return fmt.Errorf(
					"runs: apply committed cancellation of waiting child Run %q in root Run %q: %w",
					plan.target.ID(),
					plan.root.ID(),
					err,
				)
			}
			if err := change.Continue(beginCtx); err != nil {
				return fmt.Errorf(
					"runs: continue root Run %q after canceling final waiting child Run %q: %w",
					plan.root.ID(),
					plan.target.ID(),
					err,
				)
			}
			return nil
		},
	})
	if err != nil {
		if !committed.TargetRun.ID().IsZero() {
			return CancelResult{}, &CancellationSettlementError{Cause: err}
		}
		return CancelResult{}, err
	}
	if err := c.acceptedSubtreeCancellations.DeleteSubtreeCancellation(ctx, plan.root.ID(), plan.target.ID()); err != nil {
		return CancelResult{}, &CancellationSettlementError{Cause: err}
	}
	c.publishWaitingChildCancellation(plan, transformation)
	return CancelResult{Run: committed.TargetRun, RootRun: &committed.RootRun}, nil
}

func (c *Cancellations) publishWaitingChildCancellation(
	plan waitingCancellationPlan,
	transformation waitingCancellationTransformation,
) {
	affected := make([]identity.RunID, 0, len(transformation.canceledRunIDs)+len(plan.survivingRuns)+2)
	seen := make(map[identity.RunID]struct{}, cap(affected))
	appendRunID := func(runID identity.RunID) {
		if runID.IsZero() {
			return
		}
		if _, duplicate := seen[runID]; duplicate {
			return
		}
		seen[runID] = struct{}{}
		affected = append(affected, runID)
	}
	for _, runID := range transformation.canceledRunIDs {
		appendRunID(runID)
	}
	appendRunID(plan.target.Lineage().ParentRunID())
	if transformation.remaining == nil {
		for _, continuation := range transformation.continuation.continuations {
			appendRunID(continuation.RunID)
		}
	}
	appendRunID(plan.root.ID())
	c.segments.publications.publishWaitingSubtreeCanceled(
		plan.root.SessionID(),
		plan.root.ID(),
		affected,
	)
}
