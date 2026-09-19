package agentexec

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
		managed := i.state.delegateChildren[owner.Snapshot.ProcessID()]
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
			if process.Snapshot.ProcessID() != owner.Snapshot.ProcessID() && (parent != owner.Snapshot.ProcessID() || !i.state.deployments.toolChild(process.Snapshot.DeploymentRef())) {
				continue
			}
			if process.RuntimeError == nil {
				continue
			}
			for _, id := range process.RuntimeError.UnresolvedEffectIDs() {
				effect, err := run.NewUnresolvedEffect(process.Snapshot.ProcessID().String(), id.String(), "runtime_failure", executorDiagnostic(cause), executorDiagnostic(process.RuntimeError))
				if err != nil {
					return err
				}
				effects = append(effects, effect)
			}
		}
		reason, kind := run.OutcomeFailed, run.FailureInternal
		if len(effects) > 0 {
			reason, kind = run.OutcomeLost, run.FailureLost
		}
		failure := run.Failure{Kind: kind, Detail: executorDiagnostic(cause)}
		if err := i.commitFact(ctx, member, runs.NewSegmentEnded(reason, &failure, nil, 0).WithUnresolvedEffects(effects)); err != nil {
			return err
		}
	}
	return nil
}
