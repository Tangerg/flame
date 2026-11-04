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

// A stopped writer cannot invent an Exit. Flame projects its own loss only
// after Runtime.Close has joined external work and retained pending evidence.
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
	members := i.runtimeFailureMembers(inspection)
	var stopped *scoperun.RuntimeError
	if !errors.As(cause, &stopped) {
		return cause
	}
	for _, member := range members {
		owner, _ := scoperun.ParseProcessID(member.MemberID)
		i.state.mu.Lock()
		managed := i.state.delegateChildren[owner]
		i.state.mu.Unlock()
		if managed != nil {
			managed.mu.Lock()
			projected := managed.segmentProjected
			managed.mu.Unlock()
			if projected {
				continue
			}
		}
		end, err := i.runtimeFailureEnd(member, cause, stopped)
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
			if pending.Failure != nil {
				i.effectFailures.record(pending.Request.ID(), errors.New(pending.Failure.Message()))
				continue
			}
			if _, known := i.state.deployments.interactions[process.Program]; !known || !pending.Outcome.Valid() {
				continue
			}
			if _, model, err := interaction.ModelRequestFromEffect(pending.Request.Effect()); err != nil {
				return err
			} else if model {
				settlement, err := interaction.ModelSettlementFromOutcome(pending.Outcome)
				if err != nil {
					return err
				}
				if settlement.Failure != "" {
					i.effectFailures.record(pending.Request.ID(), errors.New(settlement.Failure))
				}
			}
		}
	}
	return nil
}

func (i *interactionSession) runtimeFailureMembers(inspection scoperun.TreeInspection) []runs.ExecutorMember {
	owners := make(map[string]runs.ExecutorMember)
	for _, process := range inspection.Processes {
		member, found := i.executorMemberByProcessID(process.ID)
		if found {
			owners[member.MemberID] = member
		}
	}
	members := make([]runs.ExecutorMember, 0, len(owners))
	for _, owner := range owners {
		members = append(members, owner)
	}
	slices.SortFunc(members, func(a, b runs.ExecutorMember) int {
		left, _ := scoperun.ParseProcessID(a.MemberID)
		right, _ := scoperun.ParseProcessID(b.MemberID)
		return int(right.Depth()) - int(left.Depth())
	})
	return members
}

func (i *interactionSession) runtimeFailureEnd(member runs.ExecutorMember, cause error, stopped *scoperun.RuntimeError) (runs.SegmentEnded, error) {
	owner, _ := scoperun.ParseProcessID(member.MemberID)
	var effects []run.UnresolvedEffect
	uncertainSubtree := false
	for _, id := range stopped.UnresolvedEffectIDs() {
		uncertainSubtree = uncertainSubtree || owner.Contains(id.Process)
		source, found := i.executorMemberByProcessID(id.Process)
		if !found || source.MemberID != member.MemberID {
			continue
		}
		observation := i.effectFailures.observations([]scoperun.EffectID{id})[0]
		if observation.Detail == "" {
			observation.Detail = run.Diagnostic(stopped.Unwrap())
		}
		effect, err := run.NewUnresolvedEffect(run.UnresolvedEffectConfig{
			ProcessID: id.Process.String(), EffectID: id.String(), Cause: "runtime_failure",
			Reason: run.Diagnostic(cause), Detail: observation.Detail, Output: observation.Output,
		})
		if err != nil {
			return runs.SegmentEnded{}, err
		}
		effects = append(effects, effect)
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
