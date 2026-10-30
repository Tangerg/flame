package execution

import (
	"context"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
)

// A durability failure stops the runtime without inventing a Scope terminal.
// Project its retained uncertainty only after Join has drained external work.
func (i *interactionSession) publishRuntimeFailure(cause error) error {
	ctx, cancel := i.lifetime.publicationContext(context.Background())
	defer cancel()
	inspection, err := i.engine.InspectTree(ctx, i.processRootID())
	if err != nil {
		return err
	}
	for index := len(inspection.Processes) - 1; index >= 0; index-- {
		owner := inspection.Processes[index]
		if i.state.deployments.toolChild(owner.Snapshot.DeploymentRef()) {
			continue
		}
		member := i.executorMember(owner.Snapshot.Relation())
		i.state.mu.Lock()
		managed := i.state.delegateChildren[owner.Snapshot.Relation().ProcessID()]
		i.state.mu.Unlock()
		if managed != nil {
			managed.mu.Lock()
			projected := managed.segmentProjected
			managed.mu.Unlock()
			if projected {
				continue
			}
		}
		var effects []run.UnresolvedEffect
		for _, process := range inspection.Processes {
			parent, _ := process.Snapshot.Relation().ParentID()
			if process.Snapshot.Relation().ProcessID() != owner.Snapshot.Relation().ProcessID() && (parent != owner.Snapshot.Relation().ProcessID() || !i.state.deployments.toolChild(process.Snapshot.DeploymentRef())) {
				continue
			}
			if process.RuntimeError == nil {
				continue
			}
			for _, observation := range i.effectFailures.observations(process.RuntimeError.UnresolvedEffectIDs()) {
				effect, err := run.NewUnresolvedEffect(run.UnresolvedEffectConfig{
					ProcessID: process.Snapshot.Relation().ProcessID().String(), EffectID: observation.ID, Cause: "runtime_failure",
					Reason: executorDiagnostic(cause), Detail: executorDiagnostic(process.RuntimeError), Output: observation.Output,
				})
				if err != nil {
					return err
				}
				effects = append(effects, effect)
			}
		}
		var ending run.Ending = run.Failed{Failure: run.Failure{Kind: run.FailureInternal, Detail: executorDiagnostic(cause)}}
		if len(effects) > 0 {
			ending = run.Lost{Loss: run.Loss{Reason: run.LossUnresolvedEffects, Detail: executorDiagnostic(cause)}}
		}
		if err := i.commitFact(ctx, member, runs.NewSegmentEnded(ending, 0).WithUnresolvedEffects(effects)); err != nil {
			return err
		}
	}
	return nil
}
