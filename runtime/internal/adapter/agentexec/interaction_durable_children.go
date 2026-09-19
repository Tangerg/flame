package agentexec

import (
	"errors"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	agent "github.com/Tangerg/scope/agent"
)

func (i *interactionSession) childTerminalFacts(tree agent.TreeSnapshot) ([]runs.ExecutorEvent, []*managedDelegateCall, error) {
	snapshots := tree.ProcessSnapshots()
	parents := capturedParents(tree)
	var facts []runs.ExecutorEvent
	var children []*managedDelegateCall
	for index := len(snapshots) - 1; index >= 0; index-- {
		snapshot := snapshots[index]
		i.state.mu.Lock()
		managed := i.state.delegateChildren[snapshot.ProcessID()]
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
		result, complete := snapshot.Result()
		if !complete {
			continue
		}
		settled := true
		for _, child := range snapshots {
			if descendsFrom(parents, child.ProcessID(), snapshot.ProcessID()) && (!child.Status().Terminal() || len(child.UnknownEffectIDs()) > 0) {
				settled = false
				break
			}
		}
		if !settled {
			continue
		}
		member := i.executorMember(snapshot.Relation())
		if result.Status() == agent.StatusCompleted {
			reply, found := i.committedReplies.lookup(result.ProcessID())
			if !found {
				return nil, nil, errors.New("agentexec: completed child has no committed model reply")
			}
			if !messageRequestsTools(reply) {
				completion, err := runs.NewAssistantMessageCompleted(reply)
				if err != nil {
					return nil, nil, err
				}
				facts = append(facts, runs.ExecutorEvent{Member: member, Payload: completion})
			}
		}
		draft := segmentEndFromTermination(result.Termination(), i.segmentClock.duration(result.StartedAt(), result.FinishedAt()))
		usage, err := i.accounting.segmentUsage(result.ProcessID())
		if err != nil {
			return nil, nil, err
		}
		facts = append(facts, runs.ExecutorEvent{Member: member, Payload: runs.NewSegmentEnded(draft.reason, draft.failure, usage, draft.duration)})
		children = append(children, managed)
	}
	return facts, children, nil
}
