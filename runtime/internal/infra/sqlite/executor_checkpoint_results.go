package sqlite

import (
	"context"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/domain/run"

	"github.com/Tangerg/flame/runtime/internal/domain/run/toolresult"
)

// References and payload advance in the same checkpoint transaction. A waiting
// checkpoint is a durable owner even before result publication binds an Item.
func (e *ExecutorCheckpointStore) replaceToolResultReferences(ctx context.Context, checkpoint run.Checkpoint) error {
	if _, err := conn(ctx, e.db).ExecContext(ctx, `DELETE FROM executor_checkpoint_tool_results WHERE root_member_id = ?`, checkpoint.RootMemberID()); err != nil {
		return err
	}
	for _, id := range checkpoint.ToolResultIDs() {
		result, err := conn(ctx, e.db).ExecContext(ctx, `INSERT INTO executor_checkpoint_tool_results(root_member_id, result_id) SELECT ?, id FROM tool_result_blobs WHERE id = ? AND session_id = ?`, checkpoint.RootMemberID(), id, checkpoint.Scope().SessionID)
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

func (e *ExecutorCheckpointStore) toolResultReferences(ctx context.Context, rootID string) ([]toolresult.ID, error) {
	rows, err := conn(ctx, e.db).QueryContext(ctx, `SELECT result_id FROM executor_checkpoint_tool_results WHERE root_member_id = ? ORDER BY result_id`, rootID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []toolresult.ID
	for rows.Next() {
		var id toolresult.ID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
