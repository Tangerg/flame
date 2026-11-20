package goals

import (
	"context"
	"errors"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	"github.com/Tangerg/flame/packages/go-sdk/runoutcome"
	"github.com/Tangerg/flame/runtime/internal/application/invalidation"

	"github.com/Tangerg/flame/packages/go-sdk/goalworkflow"
	"github.com/Tangerg/flame/runtime/internal/domain/automation/goal"
	"github.com/samber/lo"
)

type RunEvidence interface {
	PendingGoalRun(context.Context, identity.SessionID) (goal.RunReceipt, bool, error)
	SettleGoalRun(context.Context, goal.RunReceipt, *goal.Replacement) error
}

// RunSettler turns captured terminal Goal Runs into Goal decisions.
type RunSettler struct {
	evidence RunEvidence
	publish  invalidation.Publish
}

func NewRunSettler(evidence RunEvidence, publish invalidation.Publish) (*RunSettler, error) {
	if lo.IsNil(evidence) || publish == nil {
		return nil, errors.New("goals: terminal Run evidence and invalidations are required")
	}
	return &RunSettler{evidence: evidence, publish: publish}, nil
}

// Drain never holds the Run's terminal transaction across a plugin call. A
// restart repeats an unacknowledged decision against the same immutable Run.
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
		if exists && receipt.Owns(current) {
			next, changed, err := decideGoal(ctx, workflow, current.Snapshot(), goalworkflow.Request{Signal: goalworkflow.TerminalRun, Outcome: runoutcome.Kind(receipt.Outcome)}, current.IncarnationID(), receipt.FinishedAt)
			if err != nil {
				return err
			}
			if changed {
				decided, err := goal.NewReplacement(current.Version(), next)
				if err != nil {
					return err
				}
				change = &decided
			}
		}
		if err := r.evidence.SettleGoalRun(ctx, receipt, change); err != nil {
			return err
		}
		if change != nil {
			r.publish.Notify(invalidation.InSession(invalidation.Goals, sessionID))
		}
	}
}
