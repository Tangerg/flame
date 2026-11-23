package sessions

import (
	"context"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	"github.com/Tangerg/flame/runtime/internal/domain/session/plan"
)

type PlanServices struct {
	Boundaries   planBoundaries
	Replacements *PlanCoordinator
}

func (c *Coordinator) boundaryPlanReplacement(ctx context.Context, sessionID identity.SessionID, runID identity.RunID) (*plan.Replacement, error) {
	doc := plan.Document{Steps: []plan.Step{}}
	if !runID.IsZero() {
		recorded, found, err := c.plan.Boundaries.Boundary(ctx, runID)
		if err != nil || !found {
			return nil, err
		}
		doc = recorded
	}
	current, err := c.plan.Replacements.State(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	replacement, err := c.plan.Replacements.reference(current, doc)
	if err != nil {
		return nil, err
	}
	return &replacement, nil
}

func (c *Coordinator) prepareInitialPlanReplacement(doc plan.Document) (*plan.Replacement, error) {
	if doc.Steps == nil {
		return nil, nil
	}
	replacement, err := c.plan.Replacements.reference(plan.Current{}, doc)
	if err != nil {
		return nil, err
	}
	return &replacement, nil
}

func (c *Coordinator) prepareRestoredPlanReplacement(
	ctx context.Context,
	sessionID identity.SessionID,
	doc plan.Document,
) (*plan.Replacement, error) {
	if doc.Steps == nil {
		return nil, nil
	}
	replacement, err := c.plan.Replacements.prepareAuthorDecision(ctx, sessionID, doc)
	if err != nil {
		return nil, err
	}
	return &replacement, nil
}
