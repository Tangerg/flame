package agentexec

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	agent "github.com/Tangerg/scope/agent"
)

// PrepareWaitingSubtreeCancellation projects one exact waiting Interaction tree
// as this cancellation would leave it, and claims the tree's boundary until the
// returned Application capability is applied or discarded. It changes nothing:
// the cancellation is submitted by Apply, once the Application has committed it.
func (i *InteractionExecutor) PrepareWaitingSubtreeCancellation(
	ctx context.Context,
	request runs.WaitingSubtreeCancellationRequest,
) (runs.PreparedWaitingSubtreeCancellation, error) {
	if err := request.Validate(); err != nil {
		return runs.PreparedWaitingSubtreeCancellation{}, err
	}
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
			"agentexec: waiting subtree preparation context is required",
		)
	}
	if _, bounded := ctx.Deadline(); !bounded {
		return runs.PreparedWaitingSubtreeCancellation{}, errors.New(
			"agentexec: waiting subtree preparation requires a deadline",
		)
	}
	targetID, err := agent.ParseProcessID(memberID)
	if err != nil {
		return runs.PreparedWaitingSubtreeCancellation{}, fmt.Errorf(
			"agentexec: parse waiting subtree member: %w",
			err,
		)
	}
	i.state.mu.Lock()
	if i.state.finished || i.state.process == nil {
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
	case i.state.observerWasAttached:
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
	rootID := i.state.process.Relation().RootID()
	managed := i.state.delegateChildren[targetID]
	if managed == nil {
		i.state.mu.Unlock()
		return runs.PreparedWaitingSubtreeCancellation{}, errors.New("agentexec: waiting subtree target has no delegate binding")
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
		return runs.PreparedWaitingSubtreeCancellation{}, errors.New("agentexec: waiting subtree target is unavailable")
	}
	discard := true
	defer func() {
		if discard {
			_ = i.discardPreparedSubtree(context.WithoutCancel(ctx))
		}
	}()
	stagedTree, err := i.stagedTree()
	if err != nil {
		i.failSubtreePreparation(preparedSignal)
		return runs.PreparedWaitingSubtreeCancellation{}, err
	}
	canceled, paused := i.partitionCapturedSubtree(stagedTree, targetID)
	resultingTree, err := i.engine.CaptureTree(ctx, rootID)
	if err != nil {
		i.failSubtreePreparation(preparedSignal)
		return runs.PreparedWaitingSubtreeCancellation{}, fmt.Errorf(
			"agentexec: capture waiting Interaction subtree: %w",
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
	pausedMembers, err := i.executorMemberIDs(paused)
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
	managed.mu.Lock()
	// The target is the host-canceled root of this prepared subtree. Descendants
	// carry parent cancellation, but their spawning Tools have no surviving owner.
	parentResult := delegateFailureModelResult(managed.call, delegateTerminationDiagnostic(
		agent.StatusCanceled, agent.TerminationCauseHostCancellation, reason,
	))
	managed.mu.Unlock()
	prepared, err := runs.NewPreparedWaitingSubtreeCancellation(
		canceledMembers,
		pausedMembers,
		interruptions,
		checkpoint,
		parentResult,
		change,
	)
	if err != nil {
		i.failSubtreePreparation(preparedSignal)
		return runs.PreparedWaitingSubtreeCancellation{}, fmt.Errorf(
			"agentexec: build prepared waiting subtree cancellation: %w",
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
	processIDs []agent.ProcessID,
) ([]string, error) {
	members := make([]string, len(processIDs))
	for index, processID := range processIDs {
		member, found := i.executorMemberByProcessID(processID)
		if !found || member.MemberID != processID.String() {
			return nil, fmt.Errorf(
				"agentexec: Interaction Process %s has no exact product member",
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
	if i.state.finished || i.state.boundary != interactionBoundarySubtreePreparing ||
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

// partitionCapturedSubtree reads the outcome of a requested cancellation out of
// the cut that captured it. A member the cancellation took is terminal in the
// cut; every other member was quiesced by the capture and is the set Continue
// has to resume. The target itself is always the canceled root of this subtree.
// partitionCapturedSubtree splits the product members of one captured cut into
// the target's subtree, which the accepted cancellation ends, and the members it
// leaves quiesced. The cut freezes the tree at a Strategy-safe boundary while
// that cancellation is still draining, so lineage states which side a member is
// on and a status read would only describe how far the drain had got. A Tool
// call's child Process is not a member of its own: it travels with the member
// that called it, whichever side that member lands on.
func (i *interactionSession) partitionCapturedSubtree(
	tree agent.TreeSnapshot,
	targetID agent.ProcessID,
) (canceled []agent.ProcessID, quiesced []agent.ProcessID) {
	i.state.mu.Lock()
	deployments := i.state.deployments
	i.state.mu.Unlock()
	parents := capturedParents(tree)
	canceled = make([]agent.ProcessID, 0)
	quiesced = make([]agent.ProcessID, 0)
	for _, snapshot := range tree.ProcessSnapshots() {
		if deployments != nil && deployments.toolChild(snapshot.DeploymentRef()) {
			continue
		}
		processID := snapshot.ProcessID()
		if descendsFrom(parents, processID, targetID) {
			canceled = append(canceled, processID)
			continue
		}
		quiesced = append(quiesced, processID)
	}
	slices.SortFunc(canceled, func(left, right agent.ProcessID) int {
		return strings.Compare(left.String(), right.String())
	})
	slices.SortFunc(quiesced, func(left, right agent.ProcessID) int {
		return strings.Compare(left.String(), right.String())
	})
	return canceled, quiesced
}

func capturedParents(tree agent.TreeSnapshot) map[agent.ProcessID]agent.ProcessID {
	parents := make(map[agent.ProcessID]agent.ProcessID, len(tree.ProcessSnapshots()))
	for _, snapshot := range tree.ProcessSnapshots() {
		if parentID, child := snapshot.Relation().ParentID(); child {
			parents[snapshot.ProcessID()] = parentID
		}
	}
	return parents
}

func descendsFrom(
	parents map[agent.ProcessID]agent.ProcessID,
	processID agent.ProcessID,
	ancestorID agent.ProcessID,
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
