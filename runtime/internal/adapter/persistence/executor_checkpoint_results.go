package persistence

import (
	"context"
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/sqlite"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
)

// References and payload advance in the same checkpoint transaction. A waiting
// checkpoint is a durable owner even before result publication binds an Item.
func (s *ExecutorCheckpointStore) replaceToolResultReferences(ctx context.Context, checkpoint executorCheckpointRecord) error {
	if _, err := sqlite.Connection(ctx, s.db).ExecContext(ctx, `DELETE FROM executor_checkpoint_tool_results WHERE root_member_id = ?`, checkpoint.RootMemberID); err != nil {
		return err
	}
	for _, id := range checkpoint.ToolResultIDs {
		result, err := sqlite.Connection(ctx, s.db).ExecContext(ctx, `INSERT INTO executor_checkpoint_tool_results(root_member_id, session_id, result_id) SELECT ?, session_id, id FROM tool_result_blobs WHERE id = ? AND session_id = ?`, checkpoint.RootMemberID, id, checkpoint.SessionID)
		if err != nil {
			return err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if count != 1 {
			return fmt.Errorf("sqlite: checkpoint result %q is missing or belongs to another session", id)
		}
	}
	return nil
}

func (s *ExecutorCheckpointStore) toolResultReferences(ctx context.Context, rootID string) ([]transcript.ToolResultID, error) {
	rows, err := sqlite.Connection(ctx, s.db).QueryContext(ctx, `SELECT result_id FROM executor_checkpoint_tool_results WHERE root_member_id = ? ORDER BY result_id`, rootID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []transcript.ToolResultID
	for rows.Next() {
		var id transcript.ToolResultID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
