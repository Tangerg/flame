package persistence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/automation/goal"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/qimu/sqlite"
)

func (g *GoalStore) CaptureGoalRun(ctx context.Context, value run.Run) error {
	if value.GoalIncarnationID() == "" {
		return nil
	}
	if _, terminal := value.Outcome(); !terminal {
		return errors.New("goal: cannot capture a nonterminal Run")
	}
	_, err := sqlite.Connection(ctx, g.db).ExecContext(ctx, `INSERT INTO goal_run_receipts(run_id,expected_revision)
		SELECT r.run_id,g.revision FROM runs r JOIN goals g ON g.session_id=r.session_id AND g.incarnation_id=r.goal_incarnation_id
		WHERE r.run_id=? AND r.session_id=? AND r.goal_incarnation_id=? AND r.state=?
		ON CONFLICT(run_id) DO NOTHING`, value.ID(), value.SessionID(), value.GoalIncarnationID(), run.Finished)
	if err != nil {
		return fmt.Errorf("sqlite: capture Goal Run receipt: %w", err)
	}
	return nil
}

func (g *GoalStore) PendingGoalRun(ctx context.Context, sessionID string) (goal.RunReceipt, bool, error) {
	var r goal.RunReceipt
	var finished int64
	err := sqlite.Connection(ctx, g.db).QueryRowContext(ctx, `SELECT r.run_id,r.session_id,r.goal_incarnation_id,receipt.expected_revision,r.outcome,r.finished_at
		FROM goal_run_receipts receipt JOIN runs r ON r.run_id=receipt.run_id
		WHERE r.session_id=? AND receipt.consumed=0 ORDER BY r.finished_at,r.run_id LIMIT 1`, sessionID).
		Scan(&r.RunID, &r.SessionID, &r.IncarnationID, &r.Revision, &r.Outcome, &finished)
	if errors.Is(err, sql.ErrNoRows) {
		return goal.RunReceipt{}, false, nil
	}
	if err != nil {
		return goal.RunReceipt{}, false, fmt.Errorf("sqlite: next Goal Run receipt: %w", err)
	}
	r.FinishedAt = time.Unix(0, finished).UTC()
	return r, true, nil
}

func (g *GoalStore) SettleGoalRun(ctx context.Context, receipt goal.RunReceipt, change *goal.Replacement) error {
	return sqlite.RunInTx(ctx, g.db, func(ctx context.Context) error {
		result, err := sqlite.Connection(ctx, g.db).ExecContext(ctx, `UPDATE goal_run_receipts SET consumed=1
			WHERE run_id=? AND expected_revision=? AND consumed=0 AND EXISTS(
				SELECT 1 FROM runs r WHERE r.run_id=goal_run_receipts.run_id AND r.session_id=? AND r.goal_incarnation_id=?)`, receipt.RunID, receipt.Revision, receipt.SessionID, receipt.IncarnationID)
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err != nil || n == 0 || change == nil {
			return err
		}
		expected := change.ExpectedVersion()
		id, exists := expected.IncarnationID()
		revision, ok := expected.Revision()
		if !exists || !ok || expected.SessionID() != receipt.SessionID || id != receipt.IncarnationID || revision != receipt.Revision {
			return errors.New("goal: receipt and decision address different versions")
		}
		// A competing user transition supersedes this receipt. A failed database
		// write rolls back the acknowledgment in the enclosing transaction.
		_, err = g.Save(ctx, *change)
		return err
	})
}
