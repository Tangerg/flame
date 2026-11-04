package execution

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"

	scoperun "github.com/Tangerg/scope/agent/run"
)

func (i *interactionSession) sendExecutorRequest(
	ctx context.Context,
	event runs.ExecutorEvent,
) error {
	ctx, cancel := i.lifetime.bind(ctx)
	defer cancel()
	return i.lifetime.sendAuthoritative(ctx, event)
}

// reconcileCompletedDelegateChildren projects terminal children in postorder.
// Event listeners only wake this check; public Process state is authoritative.
func (i *interactionSession) reconcileCompletedDelegateChildren(
	ctx context.Context,
) (bool, error) {
	i.childProjection.lock()
	defer i.childProjection.unlock()
	i.state.mu.Lock()
	calls := make([]*managedDelegateCall, 0, len(i.state.delegateChildren))
	for _, managed := range i.state.delegateChildren {
		calls = append(calls, managed)
	}
	i.state.mu.Unlock()
	if len(calls) == 0 {
		return false, nil
	}
	slices.SortFunc(calls, func(left, right *managedDelegateCall) int {
		if depth := int(right.parentRelation.Depth()+1) - int(left.parentRelation.Depth()+1); depth != 0 {
			return depth
		}
		if parent := strings.Compare(left.identity.parentID.String(), right.identity.parentID.String()); parent != 0 {
			return parent
		}
		if left.modelCallSequence != right.modelCallSequence {
			return cmp.Compare(left.modelCallSequence, right.modelCallSequence)
		}
		if left.toolCallIndex != right.toolCallIndex {
			return cmp.Compare(left.toolCallIndex, right.toolCallIndex)
		}
		return strings.Compare(left.childProcessID.String(), right.childProcessID.String())
	})
	// One inspection decides which delegated children have finished. Asking each
	// child separately would compose the answer out of readings taken at
	// different moments, and this reconciliation publishes terminal facts.
	inspection, candidate, err := i.inspectCompletedDelegates(ctx, calls)
	if err != nil {
		return false, fmt.Errorf("execution: inspect delegated terminals: %w", err)
	}
	if !candidate {
		return false, nil
	}
	progressed := false
	for _, managed := range calls {
		managed.mu.Lock()
		processID := managed.childProcessID
		done := managed.segmentProjected

		managed.mu.Unlock()
		if done || !processID.Valid() {
			continue
		}
		member, inspected := inspection.Process(processID)
		if !inspected || !member.Status.Terminal() || !subtreeDrained(inspection, processID) {
			continue
		}
		process, found := i.engine.Process(processID)
		if !found {
			continue
		}
		result, err := process.Await(ctx)
		if err != nil {
			return progressed, fmt.Errorf("execution: await delegated child %s: %w", processID, err)
		}
		projected, err := i.projectDelegateTerminal(ctx, managed, result)
		if err != nil {
			return progressed, err
		}
		progressed = progressed || projected

	}
	return progressed, nil
}

func (i *interactionSession) inspectCompletedDelegates(
	ctx context.Context,
	calls []*managedDelegateCall,
) (scoperun.TreeInspection, bool, error) {
	targets := make(map[scoperun.ProcessID]struct{}, len(calls))
	for _, managed := range calls {
		managed.mu.Lock()
		if !managed.segmentProjected && managed.childProcessID.Valid() {
			targets[managed.childProcessID] = struct{}{}
		}
		managed.mu.Unlock()
	}
	if len(targets) == 0 {
		return scoperun.TreeInspection{}, false, nil
	}
	history, err := i.committedTree(ctx, i.processRootID())
	if err != nil || !hasDelegateExit(history, targets) {
		return scoperun.TreeInspection{}, false, err
	}
	inspection, err := i.logCache.inspect(history)
	return inspection, true, err
}

func hasDelegateExit(history scoperun.Log, targets map[scoperun.ProcessID]struct{}) bool {
	for _, entry := range history.Entries {
		switch record := entry.Record.(type) {
		case scoperun.Checkpoint:
			return true
		case scoperun.Exited:
			if _, found := targets[record.Process]; found {
				return true
			}
		}
	}
	return false
}

func (i *interactionSession) projectDelegateTerminal(
	ctx context.Context,
	managed *managedDelegateCall,
	result scoperun.Result[scoperun.Payload],
) (bool, error) {
	managed.mu.Lock()
	if result.ProcessID() != managed.childProcessID {
		managed.mu.Unlock()
		return false, errors.New("execution: delegated result changed child identity")
	}
	if managed.segmentProjected {
		managed.mu.Unlock()
		return false, nil
	}
	parent, callID := managed.parentRelation, managed.call.ID
	managed.mu.Unlock()
	member := runs.ExecutorMember{
		MemberID: result.ProcessID().String(), ParentID: i.executorMember(parent).MemberID,
		SpawnCallID: callID,
	}
	end, err := i.segmentEnd(result)
	if err != nil {
		return false, err
	}
	if err := i.commitFact(context.WithoutCancel(ctx), member, end); err != nil {
		return false, fmt.Errorf("execution: publish delegated child terminal: %w", err)
	}
	managed.mu.Lock()
	managed.segmentProjected = true
	managed.mu.Unlock()
	i.modelFailures.forget(result.ProcessID())
	return true, nil
}
