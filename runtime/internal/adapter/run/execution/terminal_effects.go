package execution

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
	scoperun "github.com/Tangerg/scope/agent/run"
)

func hasUnknownEffects(process scoperun.ProcessInspection) bool {
	for _, pending := range process.PendingEffects {
		if pending.Outcome.Status() == scoperun.SettlementStatusUnknown {
			return true
		}
	}
	return false
}

func unresolvedProcessEffects(process scoperun.ProcessInspection) []scoperun.EffectID {
	var ids []scoperun.EffectID
	for _, pending := range process.PendingEffects {
		if !pending.Outcome.Valid() || pending.Outcome.Status() == scoperun.SettlementStatusUnknown {
			ids = append(ids, pending.Request.ID())
		}
	}
	return ids
}

// A completed Scope process has no unresolved effects. A stopped writer's
// pending intents stay available as evidence for the owning Flame Run.
func (i *interactionSession) terminalEffects(ctx context.Context, owner scoperun.ProcessID) ([]run.UnresolvedEffect, error) {
	inspection, err := i.inspectCommittedTree(ctx)
	if err != nil {
		return nil, fmt.Errorf("execution: inspect terminal effects: %w", err)
	}
	var effects []run.UnresolvedEffect
	for _, process := range inspection.Processes {
		member, found := i.executorMemberByProcessID(process.ID)
		if !found || member.MemberID != owner.String() {
			continue
		}
		for _, observation := range i.effectFailures.observations(unresolvedProcessEffects(process)) {
			evidence, err := run.NewUnresolvedEffect(run.UnresolvedEffectConfig{
				ProcessID: process.ID.String(), EffectID: observation.ID, Cause: "runtime_failure",
				Reason: run.Diagnostic(errors.New("execution stopped with unresolved work")), Detail: observation.Detail, Output: observation.Output,
			})
			if err != nil {
				return nil, err
			}
			effects = append(effects, evidence)
		}
	}
	return effects, nil
}

func subtreeDrained(inspection scoperun.TreeInspection, root scoperun.ProcessID) bool {
	for _, process := range inspection.Processes {
		if root.Contains(process.ID) && !process.Status.Terminal() {
			return false
		}
	}
	return true
}
