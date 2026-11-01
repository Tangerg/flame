package goals

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/goalworkflow"
	"github.com/Tangerg/flame/runtime/internal/domain/automation/goal"
)

type Workflow interface {
	RequireGoalAuthor(context.Context, extension.ContributionSource) error
	DecideGoal(context.Context, extension.ContributionSource, goalworkflow.Request) (goalworkflow.AuthoredDecision, error)
}

func decideGoal(ctx context.Context, workflow Workflow, state goal.Snapshot, input goalworkflow.Request, incarnation string, now time.Time) (goal.Goal, bool, error) {
	if input.Signal != goalworkflow.Start {
		current := state.WorkflowValue()
		input.Current = &current
	}
	decision, err := workflow.DecideGoal(ctx, state.Author, input)
	if err != nil {
		return goal.Goal{}, false, err
	}
	if err := decisionError(decision.Decision); err != nil {
		return goal.Goal{}, false, err
	}
	if decision.Next == nil {
		return goal.Goal{}, false, nil
	}
	next, err := state.Publish(*decision.Next, decision.Source, incarnation, now)
	return next, true, err
}

func decisionError(d goalworkflow.Decision) error {
	if d.Refusal != "" && d.Next != nil {
		return errors.New("goals: contradictory workflow decision")
	}
	switch d.Refusal {
	case "":
		return nil
	case goalworkflow.NotResumable:
		return goal.ErrNotResumable
	case goalworkflow.BlockedReasonRequired:
		return goal.ErrBlockExplanationRequired
	default:
		return fmt.Errorf("%w: workflow refused %q", goal.ErrInvalid, d.Refusal)
	}
}
func transition(ctx context.Context, w Workflow, current goal.Goal, input goalworkflow.Request, incarnation string, now time.Time) (goal.Goal, error) {
	next, changed, err := decideGoal(ctx, w, current.Snapshot(), input, incarnation, now)
	if err == nil && !changed {
		return goal.Goal{}, errors.New("goals: workflow omitted the requested transition")
	}
	return next, err
}
