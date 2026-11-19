package execution

import (
	"time"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"

	scoperun "github.com/Tangerg/scope/agent/run"
)

// Scope's Exit proves all pending Effects and descendants have settled. Its
// corresponding Run terminal belongs in the same storage acknowledgement.
func (i *interactionSession) terminalFacts(tree scoperun.Log) ([]runs.ExecutorEvent, []*managedDelegateCall, error) {
	inspection, err := tree.Inspect()
	if err != nil {
		return nil, nil, err
	}
	var facts []runs.ExecutorEvent
	var children []*managedDelegateCall
	for _, snapshot := range inspection.Processes {
		if !snapshot.Status.Terminal() {
			continue
		}
		if snapshot.ID != tree.Root {
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
			children = append(children, managed)
		}
		duration := i.segmentClock.duration(snapshot.StartedAt, snapshot.FinishedAt)
		facts = append(facts, runs.ExecutorEvent{
			Member: i.executorMember(snapshot.ID), Payload: i.terminalFact(snapshot.ID, snapshot.Termination, duration),
		})
	}
	return facts, children, nil
}

func (i *interactionSession) terminalFact(processID scoperun.ProcessID, termination scoperun.Termination, duration time.Duration) runs.SegmentEnded {
	if cancellation, ok := cancellationFromTermination(termination); ok {
		return runs.NewCancellationEnded(cancellation, duration)
	}
	return runs.NewSegmentEnded(i.endingForProcess(processID, termination), duration)
}
