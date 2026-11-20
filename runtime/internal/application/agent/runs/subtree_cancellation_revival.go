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

// ReviveAcceptedSubtreeCancellations runs before ordinary Run-loss recovery.
// The reduced product tree cannot prove that Scope has retired its old members.
func (c *Cancellations) ReviveAcceptedSubtreeCancellations(ctx context.Context) error {
	accepted, err := c.acceptedSubtreeCancellations.ListSubtreeCancellations(ctx)
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

// unrevivedSubtreeCancellations are accepted cancellations whose busy Session
// kept them from reviving; Run-loss recovery leaves their trees to them.
func (c *Cancellations) unrevivedSubtreeCancellations(ctx context.Context) ([]AcceptedSubtreeCancellation, error) {
	return c.acceptedSubtreeCancellations.ListSubtreeCancellations(ctx)
}

func (c *Cancellations) SettleAcceptedSubtreeCancellation(ctx context.Context, ref ExecutorRef, rootRunID, targetRunID identity.RunID) error {
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
	revival, err := c.acceptedSubtreeRevival(ctx, retained[0], sess)
	if err != nil {
		return err
	}
	if revival.root.State().IsTerminal() {
		ref := ExecutorRef{SessionID: revival.accepted.SessionID, ExecutorID: revival.accepted.Pending.ExecutorID}
		if err := c.SettleRootCancellation(ctx, ref, revival.root.ID()); err != nil {
			return err
		}
		return revival.retire(ctx, c.acceptedSubtreeCancellations)
	}
	if err := c.prepareAcceptedSubtreeRevival(ctx, &revival); err != nil {
		return err
	}
	if revival.accepted.Disposition() == WaitingSubtreeStaysWaiting {
		return c.reviveWaitingSubtreeCancellation(ctx, revival)
	}
	return c.reviveRunningSubtreeCancellation(ctx, revival, &admission)
}

// acceptedSubtreeRevival is one retained cancellation re-read under the
// Session's run admission, together with the tree facts its revival replays.
type acceptedSubtreeRevival struct {
	accepted AcceptedSubtreeCancellation
	session  session.Session
	tree     []rundomain.Run
	root     rundomain.Run
	request  WaitingSubtreeCancellationRequest
}

func (r acceptedSubtreeRevival) retire(ctx context.Context, store acceptedSubtreeCancellationStore) error {
	return store.DeleteSubtreeCancellation(ctx, r.accepted.RootRunID(), r.accepted.TargetRunID)
}

func (c *Cancellations) acceptedSubtreeRevival(ctx context.Context, accepted AcceptedSubtreeCancellation, sess session.Session) (acceptedSubtreeRevival, error) {
	tree, err := c.runs.Tree(ctx, accepted.RootRunID())
	if err != nil {
		return acceptedSubtreeRevival{}, err
	}
	root, found := parkedRoot(accepted.RootRunID(), tree)
	if !found {
		return acceptedSubtreeRevival{}, errors.New("runs: accepted cancellation root is missing")
	}
	return acceptedSubtreeRevival{accepted: accepted, session: sess, tree: tree, root: root}, nil
}

// prepareAcceptedSubtreeRevival rebuilds the original cancellation request and
// retires the executor that accepted it before any replacement can stage.
func (c *Cancellations) prepareAcceptedSubtreeRevival(ctx context.Context, revival *acceptedSubtreeRevival) error {
	accepted := revival.accepted
	target, found := runByID(revival.tree, accepted.TargetRunID)
	if !found || !target.Canceled() || target.Lineage().RootRunID() != revival.root.ID() {
		return errors.New("runs: accepted cancellation target is missing its terminal decision")
	}
	rootMember, _ := accepted.Pending.RootContinuation()
	checkpoint, err := c.checkpoints.ReadWaitingCheckpoint(ctx, rootMember.MemberID)
	if err != nil {
		return err
	}
	continuation, err := waitingContinuationFromPending(accepted.Pending, checkpoint, revival.tree, revival.session)
	if err != nil {
		return err
	}
	revival.request, err = NewWaitingSubtreeCancellationRequest(continuation, accepted.targetMemberID(), target.Detail())
	if err != nil {
		return err
	}
	if err := c.releases.Release(ctx, revival.request.executorRef()); err != nil && !errors.Is(err, ErrExecutorNotLive) {
		return fmt.Errorf("runs: retire original executor before accepted cancellation revival: %w", err)
	}
	return nil
}

func (c *Cancellations) reviveWaitingSubtreeCancellation(ctx context.Context, revival acceptedSubtreeRevival) error {
	pending, found, err := c.interrupts.LookupOpenInterrupt(ctx, revival.root.ID())
	if err != nil {
		return err
	}
	if !found {
		return errors.New("runs: accepted waiting cancellation lost its surviving boundary")
	}
	if err := validatePendingRunTree(pending, revival.tree); err != nil {
		return err
	}
	recovered, err := c.subtreeCancellationRestorer.StageAcceptedSubtreeCancellation(ctx, revival.request, WaitingSubtreeStaysWaiting)
	if err != nil {
		return err
	}
	if err := recovered.Settle(ctx); err != nil {
		return err
	}
	return revival.retire(ctx, c.acceptedSubtreeCancellations)
}

func (c *Cancellations) reviveRunningSubtreeCancellation(ctx context.Context, revival acceptedSubtreeRevival, admission *ownership.RunAdmission) (err error) {
	accepted, sess, root := revival.accepted, revival.session, revival.root
	if err := validateAcceptedSegments(sess.ID(), accepted.Segments, revival.tree); err != nil {
		return err
	}
	items, err := pendingItems(ctx, c.items, accepted.Pending)
	if err != nil {
		return err
	}
	handoff := &treeContinuation{rootRunID: root.ID(), runs: parkedRunsByID(revival.tree), items: items}
	for _, member := range accepted.Pending.Continuations {
		if value, found := handoff.run(member.RunID); found && value.State() == rundomain.Running {
			handoff.continuations = append(handoff.continuations, member)
		}
	}
	if err := handoff.validate(); err != nil {
		return err
	}
	recovered, err := c.subtreeCancellationRestorer.StageAcceptedSubtreeCancellation(ctx, revival.request, WaitingSubtreeResumesRunning)
	if errors.Is(err, ErrSubtreeCancellationReleased) {
		return revival.retire(ctx, c.acceptedSubtreeCancellations)
	}
	if err != nil {
		return err
	}
	ref := revival.request.executorRef()
	staged := ownStagedExecution(c.releases, ref)
	defer func() { err = staged.abandon(ctx, err, "staged subtree cancellation") }()
	_, err = c.segments.openSegment(ctx, segmentSpec{
		RunID: root.ID(), SegmentID: root.ActiveSegmentID(), SessionID: sess.ID(),
		WorkspaceCWD: sess.Workspace().Path(), Isolated: sess.Isolated(), ExecutorID: ref.ExecutorID,
		Opening: treeResumption{Continuation: handoff}, admission: admission, staged: staged,
		BeginExecution: recovered.Settle,
	})
	if err != nil {
		return err
	}
	return revival.retire(ctx, c.acceptedSubtreeCancellations)
}
