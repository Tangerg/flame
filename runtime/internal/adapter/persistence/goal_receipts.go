package persistence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	"github.com/Tangerg/flame/runtime/internal/domain/automation/goal"
	"github.com/Tangerg/flame/runtime/internal/domain/automation/goalref"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
)

// A parked Segment can disappear from the Run before its Goal author
// responds. A newer boundary replaces the pending decision for that Run.
func (r *RunStore) captureGoalBoundary(ctx context.Context, value run.Run, parkedSegmentID identity.SegmentID) error {
	if value.GoalIncarnationID().IsZero() {
		return nil
	}
	_, err := r.db.connection(ctx).ExecContext(ctx, `INSERT INTO goal_run_receipts(run_id,parked_segment_id,parked_at,expected_revision)
	SELECT r.run_id, ?, CASE WHEN r.state=? THEN r.updated_at END, g.revision
	FROM runs r JOIN goals g ON g.session_id=r.session_id AND g.incarnation_id=r.goal_incarnation_id
	WHERE r.run_id=?
	ON CONFLICT(run_id) DO UPDATE SET
		parked_segment_id=excluded.parked_segment_id,
		parked_at=excluded.parked_at,
		expected_revision=excluded.expected_revision`, parkedSegmentID, run.Waiting, value.ID())
	if err != nil {
		return fmt.Errorf("sqlite: capture Goal Run %q receipt: %w", value.ID(), err)
	}
	return nil
}

func (g *GoalStore) PendingGoalRun(ctx context.Context, sessionID identity.SessionID) (goal.RunReceipt, bool, error) {
	return scopedPair(ctx, g.db.read, func(ctx context.Context) (goal.RunReceipt, bool, error) {
		var r goal.RunReceipt
		var incarnation string
		var parkedSegmentID identity.SegmentID
		var outcome string
		var occurred int64
		err := g.db.connection(ctx).QueryRowContext(ctx, `SELECT r.run_id,r.session_id,r.goal_incarnation_id,receipt.expected_revision,
			receipt.parked_segment_id,r.outcome,COALESCE(receipt.parked_at,r.finished_at) AS occurred_at
		FROM goal_run_receipts receipt JOIN runs r ON r.run_id=receipt.run_id
		WHERE r.session_id=?
		ORDER BY occurred_at,r.run_id LIMIT 1`, sessionID).
			Scan(&r.RunID, &r.SessionID, &incarnation, &r.Revision, &parkedSegmentID, &outcome, &occurred)
		if errors.Is(err, sql.ErrNoRows) {
			return goal.RunReceipt{}, false, nil
		}
		if err != nil {
			return goal.RunReceipt{}, false, fmt.Errorf("sqlite: next Goal Run receipt: %w", err)
		}
		if r.IncarnationID, err = goalref.ParseIncarnation(incarnation); err != nil {
			return goal.RunReceipt{}, false, fmt.Errorf("sqlite: decode Goal Run receipt incarnation: %w", err)
		}
		r.OccurredAt = time.Unix(0, occurred).UTC()
		if !parkedSegmentID.IsZero() {
			r.Boundary = goal.RunPark{SegmentID: parkedSegmentID}
			return r, true, nil
		}
		ending, valid := run.ParseOutcome(outcome)
		if !valid {
			return goal.RunReceipt{}, false, fmt.Errorf("sqlite: Goal Run %q receipt has invalid outcome %q", r.RunID, outcome)
		}
		r.Boundary = goal.RunEnd{Outcome: ending}
		return r, true, nil
	})
}

func (g *GoalStore) SettleGoalRun(ctx context.Context, receipt goal.RunReceipt, change *goal.Replacement) (bool, error) {
	if change != nil && !receipt.Owns(change.ExpectedVersion()) {
		return false, errors.New("goal: receipt and decision address different versions")
	}
	parked, _ := receipt.Boundary.(goal.RunPark)
	return scopedValue(ctx, g.db.write, func(ctx context.Context) (bool, error) {
		result, err := g.db.connection(ctx).ExecContext(ctx, `DELETE FROM goal_run_receipts
			WHERE run_id=? AND parked_segment_id=? AND expected_revision=? AND EXISTS(
				SELECT 1 FROM runs r WHERE r.run_id=goal_run_receipts.run_id AND r.session_id=? AND r.goal_incarnation_id=?)`,
			receipt.RunID, parked.SegmentID, receipt.Revision, receipt.SessionID, receipt.IncarnationID.String())
		if err != nil {
			return false, fmt.Errorf("sqlite: consume Goal Run %q receipt: %w", receipt.RunID, err)
		}
		consumed, err := rowsAffected(result)
		if err != nil || !consumed || change == nil {
			return false, err
		}
		return g.Save(ctx, *change)
	})
}
