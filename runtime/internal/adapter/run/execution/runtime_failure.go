package execution

import (
	"context"
	"errors"
	"slices"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/scope/agent/interaction"
	scoperun "github.com/Tangerg/scope/agent/run"
)

func (i *interactionSession) publishRuntimeFailure(cause error) error {
	ctx, cancel := i.lifetime.publicationContext(context.Background())
	defer cancel()
	if err := i.engine.Close(ctx); err != nil {
		return err
	}
	inspection, err := i.inspectCommittedTree(ctx)
	if err != nil {
		return err
	}
	if err := i.capturePendingFailures(inspection); err != nil {
		return err
	}
	var stopped *scoperun.RuntimeError
	if !errors.As(cause, &stopped) {
		return cause
	}
	for _, owner := range i.runtimeFailureOwners(inspection) {
		member, found := i.executorMemberByProcessID(owner)
		if !found || i.state.projectedDelegate(owner) {
			continue
		}
		end, err := i.runtimeFailureEnd(owner, cause, stopped)
		if err != nil {
			return err
		}
		if err := i.commitFact(ctx, member, end); err != nil {
			return err
		}
		i.modelFailures.forget(owner)
	}
	return nil
}

func (i *interactionSession) capturePendingFailures(inspection scoperun.TreeInspection) error {
	for _, process := range inspection.Processes {
		for _, pending := range process.PendingEffects {
			settled := scoperun.EffectSettled{ID: pending.Request.ID(), Outcome: pending.Outcome, Failure: pending.Failure}
			if err := i.captureSettlementFailure(process.Program, pending.Request.Effect(), settled); err != nil {
				return err
			}
		}
	}
	return nil
}

func (i *interactionSession) captureSettlementFailure(program scoperun.ProgramRef, effect scoperun.Effect, settled scoperun.EffectSettled) error {
	if settled.Failure != nil {
		i.effectFailures.record(settled.ID, errors.New(settled.Failure.Message()))
		return nil
	}
	if _, known := i.state.deployments.interactions[program]; !known || !settled.Outcome.Valid() {
		return nil
	}
	_, model, err := interaction.ModelCallFromEffect(effect)
	if err != nil || !model {
		return err
	}
	settlement, err := interaction.ModelSettlementFromRecord(settled)
	if err != nil {
		return err
	}
	if settlement.Failure != "" {
		i.effectFailures.record(settled.ID, errors.New(settlement.Failure))
	}
	return nil
}

func (i *interactionSession) runtimeFailureOwners(inspection scoperun.TreeInspection) []scoperun.ProcessID {
	seen := make(map[scoperun.ProcessID]bool)
	var owners []scoperun.ProcessID
	for _, process := range inspection.Processes {
		if owner, found := i.memberOwner(process.ID); found && !seen[owner] {
			seen[owner] = true
			owners = append(owners, owner)
		}
	}
	slices.SortFunc(owners, func(a, b scoperun.ProcessID) int { return int(b.Depth()) - int(a.Depth()) })
	return owners
}

func (i *interactionSession) runtimeFailureEnd(owner scoperun.ProcessID, cause error, stopped *scoperun.RuntimeError) (runs.SegmentEnded, error) {
	effects, uncertainSubtree, err := i.unresolvedEvidence(owner, stopped.PendingEffectIDs(), unresolvedCause{
		cause: run.UnresolvedAtRuntimeFailure, reason: run.Diagnostic(cause), fallbackDetail: run.Diagnostic(stopped.Unwrap()),
	})
	if err != nil {
		return runs.SegmentEnded{}, err
	}
	var ending run.Ending = run.Failed{Failure: run.Failure{Kind: run.FailureInternal, Detail: run.Diagnostic(cause)}}
	if uncertainSubtree {
		ending = run.Lost{Loss: run.Loss{Reason: run.LossUnresolvedEffects, Detail: run.Diagnostic(cause)}}
	}
	if classified, found := i.modelFailures.lookup(owner); found && i.lifetime.execution.Err() == nil {
		ending = run.Failed{Failure: classified}
	}
	return runs.NewSegmentEnded(ending, 0).WithUnresolvedEffects(effects), nil
}

type unresolvedCause struct {
	cause                  run.UnresolvedCause
	reason, fallbackDetail string
}

func (i *interactionSession) unresolvedEvidence(owner scoperun.ProcessID, ids []scoperun.EffectID, why unresolvedCause) ([]run.UnresolvedEffect, bool, error) {
	var effects []run.UnresolvedEffect
	uncertainSubtree := false
	for _, id := range ids {
		uncertainSubtree = uncertainSubtree || owner.Contains(id.Process)
		if source, found := i.memberOwner(id.Process); !found || source != owner {
			continue
		}
		observation := i.effectFailures.observations([]scoperun.EffectID{id})[0]
		if observation.Detail == "" {
			observation.Detail = why.fallbackDetail
		}
		effect, err := run.NewUnresolvedEffect(run.UnresolvedEffectConfig{
			ProcessID: id.Process.String(), EffectID: id.String(), Cause: why.cause,
			Reason: why.reason, Detail: observation.Detail, Output: observation.Output,
		})
		if err != nil {
			return nil, false, err
		}
		effects = append(effects, effect)
	}
	return effects, uncertainSubtree, nil
}
