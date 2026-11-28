package sessions

import (
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/flame/packages/go-sdk/exactint"
	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/Tangerg/flame/runtime/internal/domain/session/plan"
)

type ForkPlan struct {
	parentID        identity.SessionID
	parentWorkspace session.Workspace
	parentRevision  uint64
	history         HistorySnapshot
	planBoundaries  []RunPlanBoundary
	planReplacement *plan.Replacement
}

func newForkPlan(
	parent session.Session,
	expectedRevision uint64,
	material ForkMaterial,
	planReplacement *plan.Replacement,
) (ForkPlan, error) {
	parentID := parent.ID()
	if err := parentID.Validate(); err != nil {
		return ForkPlan{}, fmt.Errorf("sessions: fork plan parent: %w", err)
	}
	owned, err := ownHistory(material.HistorySnapshot)
	if err != nil {
		return ForkPlan{}, fmt.Errorf("sessions: fork plan snapshot: %w", err)
	}
	if owned.Session.ParentID() != parentID {
		return ForkPlan{}, errors.New("sessions: fork plan child belongs to a different parent")
	}
	if owned.Session.Revision() != exactint.First().Value() {
		return ForkPlan{}, errors.New("sessions: fork plan child is not at its initial revision")
	}
	boundaries, err := ownForkPlanBoundaries(owned.Runs, material.PlanBoundaries)
	if err != nil {
		return ForkPlan{}, err
	}
	if planReplacement != nil {
		if err := planReplacement.Validate(); err != nil {
			return ForkPlan{}, fmt.Errorf("sessions: fork plan replacement: %w", err)
		}
		if !planReplacement.ExpectedVersion().IsUnwritten() {
			return ForkPlan{}, errors.New("sessions: fork plan replacement does not start from an unwritten Plan")
		}
		planReplacement = new(*planReplacement)
	}
	return ForkPlan{
		parentID: parentID, parentWorkspace: parent.Workspace(), parentRevision: expectedRevision, history: owned, planBoundaries: boundaries, planReplacement: planReplacement,
	}, nil
}

func ownForkPlanBoundaries(runs []run.Run, captures []RunPlanBoundary) ([]RunPlanBoundary, error) {
	roots := make(map[identity.RunID]struct{}, len(runs))
	for _, value := range runs {
		if value.Lineage().IsRoot() {
			roots[value.ID()] = struct{}{}
		}
	}
	for _, capture := range captures {
		if _, matched := roots[capture.RunID]; !matched {
			return nil, fmt.Errorf("sessions: fork Plan capture refers to a missing or repeated root Run %q", capture.RunID)
		}
		if capture.Document.Steps == nil {
			return nil, fmt.Errorf("sessions: fork Plan capture for Run %q is missing its document", capture.RunID)
		}
		delete(roots, capture.RunID)
	}
	return clonePlanBoundaries(captures), nil
}

func (f ForkPlan) ParentID() identity.SessionID { return f.parentID }

func (f ForkPlan) ParentMatches(parent session.Session) bool {
	return parent.Workspace() == f.parentWorkspace && (f.parentRevision == 0 || parent.Revision() == f.parentRevision)
}

func (f ForkPlan) History() HistorySnapshot { return f.history.clone() }

func (f ForkPlan) PlanBoundaries() []RunPlanBoundary { return clonePlanBoundaries(f.planBoundaries) }

func (f ForkPlan) PlanReplacement() *plan.Replacement {
	if f.planReplacement == nil {
		return nil
	}
	return new(*f.planReplacement)
}

func clonePlanBoundaries(captures []RunPlanBoundary) []RunPlanBoundary {
	owned := slices.Clone(captures)
	for index := range owned {
		owned[index].Document.Steps = slices.Clone(owned[index].Document.Steps)
	}
	return owned
}
