package execution

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	agent "github.com/Tangerg/scope/agent"
)

func (i *InteractionExecutor) StageAcceptedContinuation(ctx context.Context, continuation runs.WaitingContinuation) (runs.ExecutorRef, error) {
	continuation = continuation.Clone()
	if err := continuation.Validate(); err != nil {
		return runs.ExecutorRef{}, err
	}
	if !i.acceptsBuild(continuation.Checkpoint.BuildID) {
		return runs.ExecutorRef{}, fmt.Errorf("%w: accepted continuation belongs to another build", runs.ErrExecutorStateLost)
	}
	ref := runs.ExecutorRef{SessionID: continuation.SessionID, ExecutorID: continuation.ExecutorID}
	if session, err := i.session(ref); err == nil {
		if err := session.stageAcceptedContinuation(continuation.Checkpoint); err != nil {
			return runs.ExecutorRef{}, err
		}
		return ref, nil
	} else if !errors.Is(err, runs.ErrExecutorNotLive) {
		return runs.ExecutorRef{}, err
	}
	if err := i.restoreWaitingTree(ctx, ref, continuation, interactionBoundaryAcceptedPaused); err != nil {
		return runs.ExecutorRef{}, err
	}
	return ref, nil
}

func validateAcceptedPausedTree(cut interactionHandoff, head agent.TreeSnapshot, retired map[agent.ProcessID]bool) error {
	expected := 0
	for id := range cut.progress {
		if !retired[id] {
			expected++
		}
	}
	current := 0
	for _, snapshot := range head.ProcessSnapshots() {
		if !retired[snapshot.Relation().ProcessID()] {
			current++
		}
	}
	if cut.rootID != head.RootID() || expected != current {
		return fmt.Errorf("%w: accepted tree membership changed", runs.ErrExecutorStateLost)
	}
	for _, snapshot := range head.ProcessSnapshots() {
		if retired[snapshot.Relation().ProcessID()] {
			continue
		}
		prior, found := cut.progress[snapshot.Relation().ProcessID()]
		if !found || snapshot.Usage().CommittedSteps != prior.CommittedSteps || snapshot.Usage().PreparedEffects != prior.PreparedEffects {
			return fmt.Errorf("%w: accepted tree advanced beyond its handoff", runs.ErrExecutorStateLost)
		}
		if prior.Status.Terminal() {
			if snapshot.Status() != prior.Status {
				return fmt.Errorf("%w: terminal accepted member changed", runs.ErrExecutorStateLost)
			}
			continue
		}
		if prior.Status != agent.StatusPaused || snapshot.Status() != agent.StatusPaused || len(snapshot.UnknownEffectIDs()) != 0 {
			return fmt.Errorf("%w: accepted member is outside its paused handoff", runs.ErrExecutorStateLost)
		}
	}
	return nil
}

// A committed child cancellation retires its product member before Scope drains
// the subtree. Its old waits and subsequent cancellation progress cannot constrain
// delivery or restoration of the surviving accepted handoff.
func acceptedRetiredProcesses(cut interactionHandoff, head agent.TreeSnapshot, children map[agent.ProcessID]*managedDelegateCall) map[agent.ProcessID]bool {
	retiredRoots := make(map[agent.ProcessID]bool)
	for id, child := range children {
		child.mu.Lock()
		retiredRoots[id] = child.segmentProjected
		child.mu.Unlock()
	}
	return retiredHandoffProcesses(cut, head, retiredRoots)
}

func retiredHandoffProcesses(cut interactionHandoff, head agent.TreeSnapshot, retiredRoots map[agent.ProcessID]bool) map[agent.ProcessID]bool {
	parents := capturedParents(head)
	for id, progress := range cut.progress {
		if progress.ParentID != "" {
			parent, _ := agent.ParseProcessID(progress.ParentID)
			parents[id] = parent
		}
	}
	retired := make(map[agent.ProcessID]bool)
	for id := range parents {
		for ancestor, depth := id, 0; ancestor.Valid() && depth <= len(parents); ancestor, depth = parents[ancestor], depth+1 {
			if retiredRoots[ancestor] {
				retired[id] = true
				break
			}
		}
	}
	return retired
}

func (i *interactionSession) acceptedRetiredProcesses(cut interactionHandoff, head agent.TreeSnapshot) map[agent.ProcessID]bool {
	i.state.mu.Lock()
	children := i.state.delegateChildren
	i.state.mu.Unlock()
	return acceptedRetiredProcesses(cut, head, children)
}
