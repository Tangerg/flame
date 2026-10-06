package sqlite

import (
	"context"
	"fmt"
)

const sessionFeedbackPredicate = `session_id = ?
	OR run_id IN (SELECT run_id FROM runs WHERE session_id = ?)
	OR item_id IN (SELECT item_id FROM history_items WHERE session_id = ?)`

// TrajectoryExportSize counts stored source bytes before their JSON bodies are
// decoded. Final wire encoding has a separate bound because escaping expands JSON.
func (s *SessionStore) TrajectoryExportSize(ctx context.Context, sessionID string) (records, bytes int64, err error) {
	if err := s.validateTrajectoryJournalOwnership(ctx, sessionID); err != nil {
		return 0, 0, err
	}
	query := `SELECT COALESCE(SUM(records), 0), COALESCE(SUM(bytes), 0) FROM (
		SELECT COUNT(*) records, SUM(length(CAST(title AS BLOB)) + length(CAST(workspace_path AS BLOB))
			+ length(CAST(id AS BLOB)) + length(CAST(parent_id AS BLOB)) + length(CAST(reasoning_effort AS BLOB))
			+ length(CAST(provider AS BLOB)) + length(CAST(model AS BLOB))) bytes FROM sessions WHERE id = ?
		UNION ALL SELECT COUNT(*), SUM(length(CAST(message AS BLOB))) FROM messages WHERE conversation_id = ?
		UNION ALL SELECT COUNT(*), SUM(length(CAST(payload AS BLOB)) + length(CAST(item_id AS BLOB))
			+ length(CAST(run_id AS BLOB)) + length(CAST(session_id AS BLOB))) FROM history_items WHERE session_id = ?
		UNION ALL SELECT COUNT(*), SUM(length(CAST(detail AS BLOB)) + length(CAST(problem AS BLOB))
			+ length(CAST(unresolved_effects AS BLOB)) + length(CAST(usage AS BLOB)) + length(CAST(capabilities AS BLOB))
			+ length(CAST(run_id AS BLOB)) + length(CAST(session_id AS BLOB)) + length(CAST(spawned_by_item_id AS BLOB))
			+ length(CAST(parent_run_id AS BLOB)) + length(CAST(root_run_id AS BLOB)) + length(CAST(reasoning_effort AS BLOB))
			+ length(CAST(state AS BLOB)) + length(CAST(outcome AS BLOB))
			+ length(CAST(provider AS BLOB)) + length(CAST(model AS BLOB))) FROM runs WHERE session_id = ?
		UNION ALL SELECT COUNT(*), SUM(length(CAST(body AS BLOB))
			+ length(CAST(id AS BLOB)) + length(CAST(item_id AS BLOB))) FROM tool_result_blobs WHERE session_id = ?
		UNION ALL SELECT COUNT(*), SUM(length(CAST(steps AS BLOB))) FROM session_plans WHERE session_id = ?
		UNION ALL SELECT COUNT(*), SUM(COALESCE(length(CAST(usage AS BLOB)), 0) + length(CAST(call_id AS BLOB))
			+ length(CAST(run_id AS BLOB)) + length(CAST(segment_id AS BLOB)) + length(CAST(state AS BLOB))) FROM model_invocations WHERE session_id = ?
		UNION ALL SELECT COUNT(*), SUM(length(CAST(call_id AS BLOB)) + length(CAST(item_id AS BLOB))
			+ length(CAST(run_id AS BLOB)) + length(CAST(segment_id AS BLOB)) + length(CAST(state AS BLOB))) FROM tool_invocations WHERE session_id = ?
		UNION ALL SELECT COUNT(*), SUM(length(CAST(text AS BLOB)) + length(CAST(session_id AS BLOB))
			+ length(CAST(run_id AS BLOB)) + length(CAST(item_id AS BLOB)) + length(CAST(rating AS BLOB))) FROM feedback_entries WHERE ` + sessionFeedbackPredicate + `
	)`
	args := make([]any, 11)
	for i := range args {
		args[i] = sessionID
	}
	if err = conn(ctx, s.db).QueryRowContext(ctx, query, args...).Scan(&records, &bytes); err != nil {
		err = fmt.Errorf("sqlite: measure trajectory export: %w", err)
	}
	return records, bytes, err
}

func (s *SessionStore) validateTrajectoryJournalOwnership(ctx context.Context, sessionID string) error {
	for _, table := range []string{"model_invocations", "tool_invocations"} {
		query := `SELECT EXISTS(SELECT 1 FROM ` + table + ` attempt LEFT JOIN runs owner ON owner.run_id = attempt.run_id
			WHERE (attempt.session_id = ? OR owner.session_id = ?)
			AND (owner.run_id IS NULL OR attempt.session_id != owner.session_id))`
		var invalid bool
		if err := conn(ctx, s.db).QueryRowContext(ctx, query, sessionID, sessionID).Scan(&invalid); err != nil {
			return fmt.Errorf("sqlite: inspect trajectory journal ownership: %w", err)
		}
		if invalid {
			return fmt.Errorf("sqlite: trajectory %s has inconsistent session ownership", table)
		}
	}
	return nil
}
