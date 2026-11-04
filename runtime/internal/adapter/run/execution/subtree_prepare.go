package execution

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"

	scoperun "github.com/Tangerg/scope/agent/run"
)

// PrepareWaitingSubtreeCancellation projects one exact waiting Interaction tree
// as this cancellation would leave it, and claims the tree's boundary until the
// returned Application capability is applied or discarded. It changes nothing:
// the cancellation is submitted by Apply, once the Application has committed it.
func (i *InteractionExecutor) PrepareWaitingSubtreeCancellation(
	ctx context.Context,
	request runs.WaitingSubtreeCancellationRequest,
) (runs.PreparedWaitingSubtreeCancellation, error) {
	continuation := request.Continuation()
	ref := runs.ExecutorRef{
		SessionID:  continuation.SessionID,
		ExecutorID: continuation.ExecutorID,
	}
	session, err := i.session(ref)
	if errors.Is(err, runs.ErrExecutorNotLive) {
		if restoreErr := i.restoreWaitingTree(
			ctx,
			ref,
			continuation,
			interactionBoundaryWaiting,
		); restoreErr != nil {
			return runs.PreparedWaitingSubtreeCancellation{}, restoreErr
		}
		session, err = i.session(ref)
	}
	if err != nil {
		return runs.PreparedWaitingSubtreeCancellation{}, err
	}
	return session.prepareWaitingSubtreeCancellation(
		ctx,
		continuation.Checkpoint,
		request.TargetMemberID(),
		request.Reason(),
	)
}

func (i *interactionSession) prepareWaitingSubtreeCancellation(
	ctx context.Context,
	expectedCheckpoint runs.ExecutorCheckpoint,
	memberID string,
	reason string,
) (runs.PreparedWaitingSubtreeCancellation, error) {
	if ctx == nil {
		return runs.PreparedWaitingSubtreeCancellation{}, errors.New(
			"execution: waiting subtree preparation context is required",
		)
	}
	if _, bounded := ctx.Deadline(); !bounded {
		return runs.PreparedWaitingSubtreeCancellation{}, errors.New(
			"execution: waiting subtree preparation requires a deadline",
		)
	}
	targetID, err := scoperun.ParseProcessID(memberID)
	if err != nil {
		return runs.PreparedWaitingSubtreeCancellation{}, fmt.Errorf(
			"execution: parse waiting subtree member: %w",
			err,
		)
	}
	i.state.mu.Lock()
	if i.state.phase == interactionFinished || i.state.process == nil {
		i.state.mu.Unlock()
		return runs.PreparedWaitingSubtreeCancellation{}, runs.ErrExecutorNotLive
	}
	switch {
	case i.state.boundary != interactionBoundaryWaiting:
		i.state.mu.Unlock()
		return runs.PreparedWaitingSubtreeCancellation{}, fmt.Errorf(
			"%w: Interaction tree is crossing another execution boundary",
			runs.ErrExecutionClaimed,
		)
	case i.state.observer != nil:
		i.state.mu.Unlock()
		return runs.PreparedWaitingSubtreeCancellation{}, fmt.Errorf(
			"%w: Interaction tree has an active observer",
			runs.ErrExecutionClaimed,
		)
	}
	if !executorCheckpointsEqual(i.state.waitingCheckpoint, expectedCheckpoint) {
		i.state.mu.Unlock()
		return runs.PreparedWaitingSubtreeCancellation{}, fmt.Errorf(
			"%w: live Interaction checkpoint differs from the waiting subtree request",
			runs.ErrInvalidExecutorCheckpoint,
		)
	}
	rootID := i.state.process.ID().Root()
	managed := i.state.delegateChildren[targetID]
	if managed == nil {
		i.state.mu.Unlock()
		return runs.PreparedWaitingSubtreeCancellation{}, errors.New("execution: waiting subtree target has no delegate binding")
	}
	preparedSignal := make(chan struct{})
	i.state.boundary = interactionBoundarySubtreePreparing
	i.state.subtreePrepared = preparedSignal
	i.state.mu.Unlock()

	// A requested cancellation is committed the moment it is submitted and drains
	// from there, so the executor cannot hold a tree that has already been asked
	// to cancel. Preparation therefore decides against the cut this tree is
	// already parked on, which nothing can advance while its input stays
	// unanswered, and leaves the submission itself to Apply.
	if _, found := i.engine.Process(targetID); !found {
		i.failSubtreePreparation(preparedSignal)
		return runs.PreparedWaitingSubtreeCancellation{}, errors.New("execution: waiting subtree target is unavailable")
	}
	discard := true
	defer func() {
		if discard {
			// Detached because the request may already be canceled, and bounded
			// because discardPreparedSubtree's only way out of an in-flight
			// preparation is its context: hand it one that cannot end and the
			// rollback waits forever instead of giving the cut back.
			discardCtx, cancelDiscard := context.WithTimeout(
				context.WithoutCancel(ctx), executionCleanupTimeout,
			)
			_ = i.discardPreparedSubtree(discardCtx)
			cancelDiscard()
		}
	}()
	stagedTree, err := i.stagedTree()
	if err != nil {
		i.failSubtreePreparation(preparedSignal)
		return runs.PreparedWaitingSubtreeCancellation{}, err
	}
	canceled, err := i.canceledSubtree(stagedTree, targetID)
	if err != nil {
		return runs.PreparedWaitingSubtreeCancellation{}, err
	}
	resultingTree, err := i.committedTree(ctx, rootID)
	if err != nil {
		i.failSubtreePreparation(preparedSignal)
		return runs.PreparedWaitingSubtreeCancellation{}, fmt.Errorf(
			"execution: capture waiting Interaction subtree: %w",
			err,
		)
	}
	checkpoint, err := i.executorCheckpoint(resultingTree)
	if err != nil {
		i.failSubtreePreparation(preparedSignal)
		return runs.PreparedWaitingSubtreeCancellation{}, err
	}
	canceledMembers, err := i.executorMemberIDs(canceled)
	if err != nil {
		i.failSubtreePreparation(preparedSignal)
		return runs.PreparedWaitingSubtreeCancellation{}, err
	}
	interruptions, err := i.pendingInterruptions(resultingTree)
	if err != nil {
		i.failSubtreePreparation(preparedSignal)
		return runs.PreparedWaitingSubtreeCancellation{}, err
	}
	// The cut still shows the target's own input wait, because the accepted
	// cancellation is draining rather than finished. An Interrupt owned by a
	// member this preparation ends is no longer pending: it dies with the member,
	// and only the surviving members still await an answer.
	interruptions = slices.DeleteFunc(interruptions, func(interruption runs.MemberInterruption) bool {
		return slices.Contains(canceledMembers, interruption.MemberID)
	})
	change := &interactionWaitingSubtreeChange{
		session: i, checkpoint: checkpoint.Clone(),
		targetID: targetID, reason: reason,
		canceled: slices.Clone(canceled),
	}
	prepared, err := runs.NewPreparedWaitingSubtreeCancellation(
		canceledMembers,
		interruptions,
		checkpoint,
		change,
	)
	if err != nil {
		i.failSubtreePreparation(preparedSignal)
		return runs.PreparedWaitingSubtreeCancellation{}, fmt.Errorf(
			"execution: build prepared waiting subtree cancellation: %w",
			err,
		)
	}
	if err := i.completeSubtreePreparation(preparedSignal, change); err != nil {
		return runs.PreparedWaitingSubtreeCancellation{}, err
	}
	change.armExpiration(ctx)
	discard = false
	return prepared, nil
}

func (i *interactionSession) executorMemberIDs(
	processIDs []scoperun.ProcessID,
) ([]string, error) {
	members := make([]string, len(processIDs))
	for index, processID := range processIDs {
		member, found := i.executorMemberByProcessID(processID)
		if !found || member.MemberID != processID.String() {
			return nil, fmt.Errorf(
				"execution: Interaction Process %s has no exact product member",
				processID,
			)
		}
		members[index] = member.MemberID
	}
	return members, nil
}

func (i *interactionSession) failSubtreePreparation(preparedSignal chan struct{}) {
	i.state.mu.Lock()
	if i.state.boundary == interactionBoundarySubtreePreparing &&
		i.state.subtreePrepared == preparedSignal {
		i.state.boundary = interactionBoundaryWaiting
		i.state.subtreePrepared = nil
		close(preparedSignal)
	}
	i.state.mu.Unlock()
}

func (i *interactionSession) completeSubtreePreparation(
	preparedSignal chan struct{},
	change *interactionWaitingSubtreeChange,
) error {
	i.state.mu.Lock()
	defer i.state.mu.Unlock()
	if i.state.phase == interactionFinished || i.state.boundary != interactionBoundarySubtreePreparing ||
		i.state.subtreePrepared != preparedSignal || i.state.subtreeChange != nil {
		if i.state.subtreePrepared == preparedSignal {
			i.state.subtreePrepared = nil
			close(preparedSignal)
		}
		return runs.ErrExecutorNotLive
	}
	i.state.boundary = interactionBoundarySubtreePrepared
	i.state.subtreeChange = change
	i.state.subtreePrepared = nil
	close(preparedSignal)
	return nil
}

// The waiting cut precedes cancellation. Terminal processes are historical
// evidence, not members that this cancellation can end or resume. Each task's
// inner Interaction travels with its owning product member.
func (i *interactionSession) canceledSubtree(
	tree scoperun.Log,
	targetID scoperun.ProcessID,
) ([]scoperun.ProcessID, error) {
	i.state.mu.Lock()
	deployments := i.state.deployments
	i.state.mu.Unlock()
	parents := capturedParents(tree)
	inspection, err := tree.Inspect()
	if err != nil {
		return nil, err
	}
	canceled := make([]scoperun.ProcessID, 0)
	for _, snapshot := range inspection.Processes {
		if snapshot.Status.Terminal() ||
			(deployments != nil && deployments.interactionChild(snapshot.Program)) {
			continue
		}
		processID := snapshot.ID
		if descendsFrom(parents, processID, targetID) {
			canceled = append(canceled, processID)
		}
	}
	slices.SortFunc(canceled, func(left, right scoperun.ProcessID) int {
		return strings.Compare(left.String(), right.String())
	})
	return canceled, nil
}

func capturedParents(tree scoperun.Log) map[scoperun.ProcessID]scoperun.ProcessID {
	parents := make(map[scoperun.ProcessID]scoperun.ProcessID)
	for _, entry := range tree.Entries {
		if started, ok := entry.Record.(scoperun.Started); ok {
			if parent, child := started.Process.Parent(); child {
				parents[started.Process] = parent
			}
		}
	}
	return parents
}

func descendsFrom(
	parents map[scoperun.ProcessID]scoperun.ProcessID,
	processID scoperun.ProcessID,
	ancestorID scoperun.ProcessID,
) bool {
	for range len(parents) + 1 {
		if processID == ancestorID {
			return true
		}
		parentID, child := parents[processID]
		if !child {
			return false
		}
		processID = parentID
	}
	return false
}
