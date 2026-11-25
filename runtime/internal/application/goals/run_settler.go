package goals

import (
	"context"
	"errors"

	"github.com/Tangerg/flame/packages/go-sdk/goalworkflow"
	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/domain/automation/goal"
	"github.com/samber/lo"
)

type RunEvidence interface {
	PendingGoalRun(context.Context, identity.SessionID) (goal.RunReceipt, bool, error)
	SettleGoalRun(context.Context, goal.RunReceipt, *goal.Replacement) (bool, error)
}

type RunSettler struct {
	evidence RunEvidence
	publish  invalidation.Publish
}

func NewRunSettler(evidence RunEvidence, publish invalidation.Publish) (*RunSettler, error) {
	if lo.IsNil(evidence) || publish == nil {
		return nil, errors.New("goals: Run boundary evidence and invalidations are required")
	}
	return &RunSettler{evidence: evidence, publish: publish}, nil
}

// The receipt stays pending across the workflow call so the original author
// can finish the decision after a failed write or a restart.
func (r *RunSettler) Drain(ctx context.Context, store Store, workflow Workflow, sessionID identity.SessionID) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		receipt, found, err := r.evidence.PendingGoalRun(ctx, sessionID)
		if err != nil || !found {
			return err
		}
		current, exists, err := loadGoal(ctx, store, sessionID)
		if err != nil {
			return err
		}
		var change *goal.Replacement
		if exists {
			change, err = decideRunReceipt(ctx, workflow, receipt, current)
			if err != nil {
				return err
			}
		}
		applied, err := r.evidence.SettleGoalRun(ctx, receipt, change)
		if err != nil {
			return err
		}
		if applied {
			r.publish.Notify(invalidation.InSession(invalidation.Goals, sessionID))
		}
	}
}

func decideRunReceipt(ctx context.Context, workflow Workflow, receipt goal.RunReceipt, current goal.Goal) (*goal.Replacement, error) {
	request, applies := receipt.RequestFor(current)
	if !applies {
		return nil, nil
	}
	var next goal.Goal
	var err error
	if request.Signal == goalworkflow.Pause {
		next, err = transition(ctx, workflow, current, request, current.IncarnationID(), receipt.OccurredAt)
	} else {
		var changed bool
		next, changed, err = decideGoal(ctx, workflow, current.Snapshot(), request, current.IncarnationID(), receipt.OccurredAt)
		if !changed {
			return nil, err
		}
	}
	if err != nil {
		return nil, err
	}
	replacement, err := goal.NewReplacement(current.Version(), next)
	if err != nil {
		return nil, err
	}
	return &replacement, nil
}
