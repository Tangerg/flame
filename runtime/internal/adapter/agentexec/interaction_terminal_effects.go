package agentexec

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
	agent "github.com/Tangerg/scope/agent"
)

// Ordinary Tool processes belong to their calling Run, while a Delegate owns
// its own evidence. No effect payload or private strategy state is interpreted.
func (i *interactionSession) terminalEffects(ctx context.Context, owner agent.ProcessID) ([]run.UnresolvedEffect, error) {
	inspection, err := i.engine.InspectTree(ctx, i.processRootID())
	if err != nil {
		return nil, fmt.Errorf("agentexec: inspect terminal effects: %w", err)
	}
	var effects []run.UnresolvedEffect
	for _, member := range inspection.Processes {
		snapshot := member.Snapshot
		processID := snapshot.ProcessID()
		parentID, _ := snapshot.Relation().ParentID()
		if processID != owner && (parentID != owner || !i.state.deployments.toolChild(snapshot.DeploymentRef())) {
			continue
		}
		process, found := i.engine.Process(processID)
		if !found {
			return nil, errors.New("agentexec: terminal effect process is unavailable")
		}
		result, err := process.Await(ctx)
		if err != nil {
			return nil, err
		}
		termination := result.Termination()
		for _, observation := range i.effectFailures.observations(termination.UnresolvedEffectIDs()) {
			evidence, err := run.NewUnresolvedEffect(processID.String(), observation.ID, termination.Cause().String(), executorDiagnostic(errors.New(termination.Reason())), observation.Detail)
			if err != nil {
				return nil, err
			}
			effects = append(effects, evidence)
		}
	}
	return effects, nil
}

func subtreeDrained(inspection agent.TreeInspection, root agent.ProcessID) bool {
	for _, member := range inspection.Processes {
		current := member.Snapshot.ProcessID()
		for {
			if current == root {
				if !member.Snapshot.Status().Terminal() || member.Work != agent.ProcessWorkIdle {
					return false
				}
				break
			}
			ancestor, found := inspection.Process(current)
			if !found {
				return false
			}
			parent, hasParent := ancestor.Snapshot.Relation().ParentID()
			if !hasParent {
				break
			}
			current = parent
		}
	}
	return true
}
