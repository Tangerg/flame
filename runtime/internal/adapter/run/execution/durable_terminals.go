package execution

import (
	"time"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run"

	scoperun "github.com/Tangerg/scope/agent/run"
)

// Scope's Exit proves that nothing of the process or its descendants is still
// in flight; Unknown Effects stay Unknown. Its corresponding Run terminal
// belongs in the same storage acknowledgement.
func (i *interactionSession) terminalFacts(tree scoperun.Log) ([]runs.ExecutorEvent, []*managedDelegateCall, error) {
	inspection, err := tree.Inspect()
	if err != nil {
		return nil, nil, err
	}
	var facts []runs.ExecutorEvent
	var children []*managedDelegateCall
	unknown, err := i.retainedUnknownEffects(tree)
	if err != nil {
		return nil, nil, err
	}
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
		member := i.executorMember(snapshot.ID)
		fact, err := i.terminalFact(member, snapshot.ID, snapshot.Termination, unknown, duration)
		if err != nil {
			return nil, nil, err
		}
		facts = append(facts, runs.ExecutorEvent{Member: member, Payload: fact})
	}
	return facts, children, nil
}

// Scope exits a terminated process while its Unknown Effects stay Unknown.
// Their outcome is unconfirmed, so the Run keeps that evidence and is lost
// unless a classified model failure already decided it.
func (i *interactionSession) terminalFact(member runs.ExecutorMember, processID scoperun.ProcessID, termination scoperun.Termination, unknown []scoperun.EffectID, duration time.Duration) (runs.SegmentEnded, error) {
	effects, uncertain, err := i.unresolvedEvidence(member, unknown, unresolvedCause{
		name: "termination", reason: unresolvedTerminationDetail, fallbackDetail: unresolvedTerminationDetail,
	})
	if err != nil {
		return runs.SegmentEnded{}, err
	}
	if uncertain {
		var ending run.Ending = run.Lost{Loss: run.Loss{Reason: run.LossUnresolvedEffects, Detail: unresolvedTerminationDetail}}
		if classified, found := i.modelFailures.lookup(processID); found && i.lifetime.execution.Err() == nil {
			ending = run.Failed{Failure: classified}
		}
		return runs.NewSegmentEnded(ending, duration).WithUnresolvedEffects(effects), nil
	}
	if cancellation, ok := cancellationFromTermination(termination); ok {
		return runs.NewCancellationEnded(cancellation, duration), nil
	}
	return runs.NewSegmentEnded(i.endingForProcess(processID, termination), duration), nil
}

const unresolvedTerminationDetail = "execution terminated with unresolved external effects"

// retainedUnknownEffects lists the Effects whose last settlement in history is
// Unknown and records their local diagnostics. Inspection drops such Effects
// once their exited process is reaped; the history keeps them.
func (i *interactionSession) retainedUnknownEffects(history scoperun.Log) ([]scoperun.EffectID, error) {
	last := make(map[scoperun.EffectID]scoperun.EffectSettled)
	var order []scoperun.EffectID
	for _, entry := range history.Entries {
		settled, ok := entry.Record.(scoperun.EffectSettled)
		if !ok {
			continue
		}
		if _, seen := last[settled.ID]; !seen {
			order = append(order, settled.ID)
		}
		last[settled.ID] = settled
	}
	var ids []scoperun.EffectID
	for _, id := range order {
		settled := last[id]
		if settled.Outcome.Status() != scoperun.SettlementStatusUnknown {
			continue
		}
		if intent, found := declaredIntent(history, id); found {
			program, _ := capturedProgram(history, id.Process)
			if err := i.captureSettlementFailure(program, intent.Effect, settled); err != nil {
				return nil, err
			}
		}
		ids = append(ids, id)
	}
	return ids, nil
}
