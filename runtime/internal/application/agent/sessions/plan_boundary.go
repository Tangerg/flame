package sessions

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	"github.com/Tangerg/flame/runtime/internal/domain/session/plan"
)

// PlanServices supplies boundary reads and aggregate replacement decisions.
type PlanServices struct {
	Boundaries   PlanBoundaries
	Replacements PlanReplacements
}

// PlanBoundary is a Plan recovered from a Run boundary: the value, and
// whether that boundary recorded one at all. The difference is not cosmetic — an
// unrecorded boundary must leave the live list untouched, while a recorded empty
// one must clear it, and a single nil slice cannot say which is meant.
type PlanBoundary struct {
	Steps    []plan.Step
	Recorded bool
}

// planBoundary resolves the Plan the boundary at runID held. An empty runID
// is a boundary that keeps no run at all: it predates every list this session ever
// wrote, so its value is the empty list — known, not unknown. Otherwise the answer
// is whatever that run recorded when it ended, including "nothing was recorded",
// which the caller must not turn into emptiness (an imported run's boundaries were
// never captured; see [PlanBoundaries]).
func (c *Coordinator) planBoundary(ctx context.Context, runID string) (PlanBoundary, error) {
	if runID == "" {
		return newPlanBoundary(nil, true)
	}
	if _, err := resourceid.ParseRun(runID); err != nil {
		return PlanBoundary{}, fmt.Errorf("sessions: Plan boundary: %w", err)
	}
	steps, recorded, err := c.plan.Boundaries.Boundary(ctx, runID)
	if err != nil {
		return PlanBoundary{}, err
	}
	return newPlanBoundary(steps, recorded)
}

func newPlanBoundary(steps []plan.Step, recorded bool) (PlanBoundary, error) {
	if !recorded {
		if len(steps) != 0 {
			return PlanBoundary{}, errors.New("sessions: unrecorded Plan boundary carries steps")
		}
		return PlanBoundary{}, nil
	}
	if err := plan.ValidateSteps(steps); err != nil {
		return PlanBoundary{}, fmt.Errorf("sessions: invalid Plan boundary: %w", err)
	}
	return PlanBoundary{Steps: steps, Recorded: true}, nil
}

func (c *Coordinator) prepareBoundaryPlanReplacement(
	ctx context.Context,
	sessionID string,
	boundary PlanBoundary,
) (*plan.Replacement, error) {
	if !boundary.Recorded {
		return nil, nil
	}
	replacement, err := c.plan.Replacements.PrepareReplacement(ctx, sessionID, boundary.Steps)
	if err != nil {
		return nil, err
	}
	return &replacement, nil
}

func (c *Coordinator) prepareInitialPlanReplacement(steps []plan.Step) (*plan.Replacement, error) {
	if len(steps) == 0 {
		return nil, nil
	}
	replacement, err := c.plan.Replacements.PrepareInitial(steps)
	if err != nil {
		return nil, err
	}
	return &replacement, nil
}

func (c *Coordinator) prepareRestoredPlanReplacement(
	ctx context.Context,
	sessionID string,
	steps []plan.Step,
) (*plan.Replacement, error) {
	replacement, err := c.plan.Replacements.PrepareReplacement(ctx, sessionID, steps)
	if err != nil {
		return nil, err
	}
	return &replacement, nil
}
