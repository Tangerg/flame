package execution

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"

	scoperun "github.com/Tangerg/scope/agent/run"
)

func (i *InteractionExecutor) StageAcceptedContinuation(ctx context.Context, continuation runs.ExecutionContinuation) (runs.ExecutorRef, error) {
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

func validateAcceptedPausedTree(cut interactionHandoff, head scoperun.Log, retired map[scoperun.ProcessID]bool) error {
	expected := 0
	for id := range cut.progress {
		if !retired[id] {
			expected++
		}
	}
	inspection, err := head.Inspect()
	if err != nil {
		return err
	}
	current := 0
	for _, snapshot := range inspection.Processes {
		if !retired[snapshot.ID] {
			current++
		}
	}
	if cut.rootID != head.Root || expected != current {
		return fmt.Errorf("%w: accepted tree membership changed", runs.ErrExecutorStateLost)
	}
	for _, snapshot := range inspection.Processes {
		if retired[snapshot.ID] {
			continue
		}
		prior, found := cut.progress[snapshot.ID]
		if !found || snapshot.Usage.CommittedSteps != prior.CommittedSteps || snapshot.Usage.PreparedEffects != prior.PreparedEffects {
			return fmt.Errorf("%w: accepted tree advanced beyond its handoff", runs.ErrExecutorStateLost)
		}
		if prior.Status.Terminal() {
			if snapshot.Status != prior.Status {
				return fmt.Errorf("%w: terminal accepted member changed", runs.ErrExecutorStateLost)
			}
			continue
		}
		if prior.Status != scoperun.StatusPaused || snapshot.Status != scoperun.StatusPaused || hasUnknownEffects(snapshot) {
			return fmt.Errorf("%w: accepted member is outside its paused handoff", runs.ErrExecutorStateLost)
		}
	}
	return nil
}

// A committed child cancellation retires its product member before Scope drains
// the subtree. Its old waits and subsequent cancellation progress cannot constrain
// delivery or restoration of the surviving accepted handoff.
func acceptedRetiredProcesses(cut interactionHandoff, head scoperun.Log, children map[scoperun.ProcessID]*managedDelegateCall) map[scoperun.ProcessID]bool {
	retiredRoots := make(map[scoperun.ProcessID]bool)
	for id, child := range children {
		child.mu.Lock()
		retiredRoots[id] = child.segmentProjected
		child.mu.Unlock()
	}
	return retiredHandoffProcesses(cut, head, retiredRoots)
}

func retiredHandoffProcesses(cut interactionHandoff, head scoperun.Log, retiredRoots map[scoperun.ProcessID]bool) map[scoperun.ProcessID]bool {
	parents := capturedParents(head)
	for id, progress := range cut.progress {
		if progress.ParentID != "" {
			parent, _ := scoperun.ParseProcessID(progress.ParentID)
			parents[id] = parent
		}
	}
	retired := make(map[scoperun.ProcessID]bool)
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

func (i *interactionSession) acceptedRetiredProcesses(cut interactionHandoff, head scoperun.Log) map[scoperun.ProcessID]bool {
	i.state.mu.Lock()
	children := i.state.delegateChildren
	i.state.mu.Unlock()
	return acceptedRetiredProcesses(cut, head, children)
}
