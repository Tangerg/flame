package goals

import (
	"context"
	"errors"

	"github.com/Tangerg/flame/packages/go-sdk/runoutcome"
	"github.com/Tangerg/flame/runtime/internal/application/invalidation"

	"github.com/Tangerg/flame/packages/go-sdk/goalworkflow"
	"github.com/Tangerg/flame/runtime/internal/domain/automation/goal"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/samber/lo"
)

type RunEvidence interface {
	CaptureGoalRun(context.Context, run.Run) error
	PendingGoalRun(context.Context, string) (goal.RunReceipt, bool, error)
	SettleGoalRun(context.Context, goal.RunReceipt, *goal.Replacement) error
}

type RunRecorder struct{ evidence RunEvidence }

func NewRunRecorder(evidence RunEvidence) (*RunRecorder, error) {
	if lo.IsNil(evidence) {
		return nil, errors.New("goals: terminal Run evidence is required")
	}
	return &RunRecorder{evidence: evidence}, nil
}

func (r *RunRecorder) RecordRun(ctx context.Context, value run.Run) error {
	return r.evidence.CaptureGoalRun(ctx, value)
}

// Drain never holds the Run's terminal transaction across a plugin call. A
// restart repeats an unacknowledged decision against the same immutable Run.
func (r *RunRecorder) Drain(ctx context.Context, store Store, workflow Workflow, sessionID string, publish invalidation.Publish) error {
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
			publish.Notify(invalidation.InSession(invalidation.Goals, sessionID))
		}
	}
}
