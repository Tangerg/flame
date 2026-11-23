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
	if err := i.restoreContinuationTree(ctx, ref, continuation, interactionBoundaryAccepted, nil); err != nil {
		return runs.ExecutorRef{}, err
	}
	return ref, nil
}

// Scope can acknowledge some Resume controls before a crash. The handoff
// still owns activation until a survivor prepares a new Effect, which may
// already have reached its external owner by the time of recovery.
func acceptedReleaseBoundary(cut interactionHandoff, tree scoperun.TreeInspection, retired map[scoperun.ProcessID]bool) ([]scoperun.ProcessID, bool, error) {
	if cut.rootID != tree.Root {
		return nil, false, fmt.Errorf("%w: accepted tree root changed", runs.ErrExecutorStateLost)
	}
	unmatched := 0
	for id := range cut.progress {
		if !retired[id] {
			unmatched++
		}
	}
	var paused []scoperun.ProcessID
	var releaseStarted bool
	for _, snapshot := range tree.Processes {
		if retired[snapshot.ID] {
			continue
		}
		unmatched--
		prior, found := cut.progress[snapshot.ID]
		if !found || snapshot.Usage.PreparedEffects != prior.PreparedEffects || hasUnknownEffects(snapshot) {
			return nil, false, fmt.Errorf("%w: accepted tree advanced beyond its handoff", runs.ErrExecutorStateLost)
		}
		if prior.Status.Terminal() {
			if snapshot.Status != prior.Status {
				return nil, false, fmt.Errorf("%w: terminal accepted member changed", runs.ErrExecutorStateLost)
			}
			continue
		}
		if prior.Status != scoperun.StatusPaused {
			return nil, false, fmt.Errorf("%w: accepted member was not paused at its handoff", runs.ErrExecutorStateLost)
		}
		switch snapshot.Status {
		case scoperun.StatusPaused:
			paused = append(paused, snapshot.ID)
		case scoperun.StatusRunning, scoperun.StatusWaiting:
			releaseStarted = true
		default:
			return nil, false, fmt.Errorf("%w: accepted member is outside its release boundary", runs.ErrExecutorStateLost)
		}
	}
	if unmatched != 0 {
		return nil, false, fmt.Errorf("%w: accepted tree membership changed", runs.ErrExecutorStateLost)
	}
	return paused, releaseStarted, nil
}

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
