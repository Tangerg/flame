package execution

import (
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"

	scoperun "github.com/Tangerg/scope/agent/run"
)

func (i *interactionSession) childTerminalFacts(tree scoperun.Log) ([]runs.ExecutorEvent, []*managedDelegateCall, error) {
	inspection, err := tree.Inspect()
	if err != nil {
		return nil, nil, err
	}
	snapshots := inspection.Processes
	parents := capturedParents(tree)
	var facts []runs.ExecutorEvent
	var children []*managedDelegateCall
	for index := len(snapshots) - 1; index >= 0; index-- {
		snapshot := snapshots[index]
		i.state.mu.Lock()
		managed := i.state.delegateChildren[snapshot.ID]
		i.state.mu.Unlock()
		if managed == nil {
			continue
		}
		managed.mu.Lock()
		projected := managed.segmentProjected
		managed.mu.Unlock()
		if projected {
			continue
		}
		if !snapshot.Status.Terminal() {
			continue
		}
		settled := true
		for _, child := range snapshots {
			if descendsFrom(parents, child.ID, snapshot.ID) && (!child.Status.Terminal() || hasUnknownEffects(child)) {
				settled = false
				break
			}
		}
		if !settled {
			continue
		}
		member := i.executorMember(snapshot.ID)
		draft := i.segmentEndForOwner(snapshot.ID, snapshot.Termination, i.segmentClock.duration(snapshot.StartedAt, snapshot.FinishedAt))
		facts = append(facts, runs.ExecutorEvent{Member: member, Payload: runs.NewSegmentEnded(draft.ending, draft.duration)})
		children = append(children, managed)
	}
	return facts, children, nil
}
