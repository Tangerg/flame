package agentexec

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	agent "github.com/Tangerg/scope/agent"
)

type interactionWaitingSubtreeChange struct {
	mu sync.Mutex

	session    *interactionSession
	checkpoint runs.ExecutorCheckpoint
	targetID   agent.ProcessID
	reason     string
	canceled   []agent.ProcessID
	retired    []*managedDelegateCall
	stopExpiry func() bool
	state      interactionWaitingSubtreeChangeState
}

type interactionWaitingSubtreeChangeState uint8

const (
	interactionWaitingSubtreeChangePrepared interactionWaitingSubtreeChangeState = iota
	interactionWaitingSubtreeChangeDiscarded
	interactionWaitingSubtreeChangeApplyFailed
	interactionWaitingSubtreeChangeWaiting
	interactionWaitingSubtreeChangeContinuationReady
	interactionWaitingSubtreeChangeContinued
)

func (i interactionWaitingSubtreeChangeState) String() string {
	switch i {
	case interactionWaitingSubtreeChangePrepared:
		return "prepared"
	case interactionWaitingSubtreeChangeDiscarded:
		return "discarded"
	case interactionWaitingSubtreeChangeApplyFailed:
		return "apply_failed"
	case interactionWaitingSubtreeChangeWaiting:
		return "waiting"
	case interactionWaitingSubtreeChangeContinuationReady:
		return "continuation_ready"
	case interactionWaitingSubtreeChangeContinued:
		return "continued"
	default:
		return fmt.Sprintf("unknown(%d)", i)
	}
}
func (i *interactionWaitingSubtreeChange) Apply(
	disposition runs.WaitingSubtreeDisposition,
) error {
	if !disposition.Valid() {
		return fmt.Errorf("agentexec: invalid waiting subtree disposition %q", disposition)
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.state != interactionWaitingSubtreeChangePrepared {
		return fmt.Errorf(
			"agentexec: waiting Interaction subtree change cannot be applied from state %s",
			i.state,
		)
	}
	i.session.childProjection.lock()
	defer i.session.childProjection.unlock()
	i.stopExpirationLocked()
	if err := i.session.beginSubtreeApplication(i); err != nil {
		return err
	}
	// The Application has durably committed this cancellation, so submitting it
	// to the executor happens here rather than during preparation: a requested
	// cancellation is accepted immediately and cannot be revoked, and preparation
	// must stay abandonable.
	err := i.session.cancelPreparedSubtree(i)
	if err == nil {
		i.session.commitSubtreeApplication(i)
	}
	switch {
	case err != nil:
		i.state = interactionWaitingSubtreeChangeApplyFailed
	case disposition == runs.WaitingSubtreeStaysWaiting:
		i.state = interactionWaitingSubtreeChangeWaiting
	default:
		i.state = interactionWaitingSubtreeChangeContinuationReady
	}
	i.session.finishSubtreeApplication(i, disposition, err)
	if err != nil {
		return fmt.Errorf("agentexec: apply waiting Interaction subtree: %w", err)
	}
	return nil
}

func (i *interactionWaitingSubtreeChange) Continue(ctx context.Context) error {
	if ctx == nil {
		return errors.New("agentexec: waiting Interaction subtree continuation context is required")
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.state != interactionWaitingSubtreeChangeContinuationReady {
		return fmt.Errorf(
			"agentexec: waiting Interaction subtree change cannot continue from state %s",
			i.state,
		)
	}
	i.state = interactionWaitingSubtreeChangeContinued
	// Continue is the first operation that can advance the Process tree after the
	// Application durably opened the replacement product Segment.
	i.session.segmentClock.start()
	// Which members survive this cancellation is a product fact the preparation
	// already projected. Which Processes the boundary actually holds is an
	// execution fact the installed cut owns, and only those need resuming: a
	// surviving member may have been waiting rather than paused.
	paused, err := i.session.pausedProcessIDs()
	if err == nil {
		resumeCtx, cancelResume := context.WithTimeout(ctx, authoritativeProjectionTimeout)
		err = i.session.resumePausedProcesses(resumeCtx, paused)
		cancelResume()
	}
	i.session.finishSubtreeContinuation(i, err)
	if err != nil {
		return fmt.Errorf("agentexec: continue applied waiting Interaction subtree: %w", err)
	}
	return nil
}

func (i *interactionWaitingSubtreeChange) Discard() error {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.state != interactionWaitingSubtreeChangePrepared {
		return nil
	}
	i.stopExpirationLocked()
	// Preparation only read the tree, so releasing this boundary is the whole
	// rollback: an abandoned command leaves the Interaction exactly as parked as
	// it was, with the target still waiting for its answer.
	i.state = interactionWaitingSubtreeChangeDiscarded
	i.session.finishSubtreeDiscard(i)
	return nil
}

func (i *interactionWaitingSubtreeChange) armExpiration(ctx context.Context) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.state != interactionWaitingSubtreeChangePrepared {
		return
	}
	i.stopExpiry = context.AfterFunc(ctx, func() {
		_ = i.Discard()
	})
}

// stopExpirationLocked disarms the preparation lease while i.mu is held.
func (i *interactionWaitingSubtreeChange) stopExpirationLocked() {
	if i.stopExpiry != nil {
		i.stopExpiry()
		i.stopExpiry = nil
	}
}

func (i *interactionSession) beginSubtreeApplication(
	change *interactionWaitingSubtreeChange,
) error {
	i.state.mu.Lock()
	defer i.state.mu.Unlock()
	if i.state.finished || i.state.boundary != interactionBoundarySubtreePrepared ||
		i.state.subtreeChange != change {
		return runs.ErrExecutionClaimed
	}
	managedCalls := make([]*managedDelegateCall, len(change.canceled))
	for index, processID := range change.canceled {
		managed := i.state.delegateChildren[processID]
		if managed == nil {
			return fmt.Errorf("agentexec: canceled Interaction member %s has no Delegate binding", processID)
		}
		if i.state.delegateChildren[managed.childProcessID] != managed ||
			i.state.delegateCalls[managed.identity] != managed {
			return errors.New("agentexec: canceled Delegate binding changed before subtree application")
		}
		managedCalls[index] = managed
	}
	i.state.boundary = interactionBoundarySubtreeApplying
	change.retired = managedCalls
	return nil
}

// cancelPreparedSubtree submits the committed cancellation and replaces the
// staged cut with the one the tree drained into. The Application already holds
// the cut this decision was made from; the executor needs the resulting one so a
// later resume and a restart describe the same tree.
func (i *interactionSession) cancelPreparedSubtree(
	change *interactionWaitingSubtreeChange,
) error {
	ctx, cancel := context.WithTimeout(
		context.WithoutCancel(i.lifetime.execution), authoritativeProjectionTimeout,
	)
	defer cancel()
	target, found := i.engine.Process(change.targetID)
	if !found {
		return fmt.Errorf("agentexec: waiting subtree target %s is unavailable", change.targetID)
	}
	if err := target.RequestCancellation(
		runExecutionContext(ctx, i.scope, i.start), change.reason,
	); err != nil {
		return fmt.Errorf("agentexec: cancel waiting Interaction subtree: %w", err)
	}
	// Nothing needs holding here. A tree that stays waiting is held by the
	// unanswered external input of the members that survive, and a tree that
	// resumes is exactly what the committed decision asked for.
	resultingTree, err := i.engine.CaptureTree(ctx, i.state.process.Relation().RootID())
	if err != nil {
		return fmt.Errorf("agentexec: capture canceled Interaction subtree: %w", err)
	}
	checkpoint, err := i.executorCheckpoint(resultingTree)
	if err != nil {
		return err
	}
	change.checkpoint = checkpoint
	return nil
}

func (i *interactionSession) commitSubtreeApplication(
	change *interactionWaitingSubtreeChange,
) {
	for _, managed := range change.retired {
		managed.mu.Lock()
		managed.parentToolFinished = true
		managed.assistantProjected = true
		managed.segmentProjected = true
		managed.mu.Unlock()
	}
	i.state.mu.Lock()
	defer i.state.mu.Unlock()
	i.state.waitingCheckpoint = change.checkpoint.Clone()
	for _, managed := range change.retired {
		delete(i.state.delegateChildren, managed.childProcessID)
		delete(i.state.delegateCalls, managed.identity)
		i.committedReplies.forget(managed.childProcessID)
	}
}

func (i *interactionSession) finishSubtreeApplication(
	change *interactionWaitingSubtreeChange,
	disposition runs.WaitingSubtreeDisposition,
	applyErr error,
) {
	i.state.mu.Lock()
	defer i.state.mu.Unlock()
	if i.state.subtreeChange != change || i.state.boundary != interactionBoundarySubtreeApplying {
		return
	}
	if applyErr != nil {
		i.state.subtreeChange = nil
		i.state.boundary = interactionBoundarySubtreeRecovery
		return
	}
	if disposition == runs.WaitingSubtreeStaysWaiting {
		i.state.subtreeChange = nil
		i.state.boundary = interactionBoundaryWaiting
		return
	}
	i.state.boundary = interactionBoundarySubtreeApplied
}

func (i *interactionSession) finishSubtreeContinuation(
	change *interactionWaitingSubtreeChange,
	continuationErr error,
) {
	i.state.mu.Lock()
	defer i.state.mu.Unlock()
	if i.state.subtreeChange != change || i.state.boundary != interactionBoundarySubtreeApplied {
		return
	}
	i.state.subtreeChange = nil
	if continuationErr != nil {
		i.state.boundary = interactionBoundarySubtreeRecovery
		return
	}
	i.state.boundary = interactionBoundaryInactive
	i.state.waitingCheckpoint = runs.ExecutorCheckpoint{}
}

func (i *interactionSession) finishSubtreeDiscard(change *interactionWaitingSubtreeChange) {
	i.state.mu.Lock()
	defer i.state.mu.Unlock()
	if i.state.subtreeChange != change || i.state.boundary != interactionBoundarySubtreePrepared {
		return
	}
	i.state.subtreeChange = nil
	i.state.boundary = interactionBoundaryWaiting
}

func (i *interactionSession) discardPreparedSubtree(ctx context.Context) error {
	for {
		i.state.mu.Lock()
		boundary := i.state.boundary
		preparedSignal := i.state.subtreePrepared
		change := i.state.subtreeChange
		i.state.mu.Unlock()
		if boundary == interactionBoundarySubtreePreparing && preparedSignal != nil {
			select {
			case <-preparedSignal:
				continue
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		if change == nil {
			return nil
		}
		return change.Discard()
	}
}
