package sqlite

import "context"

// PendingCheckpointPayloads transfers opaque continuations. Only the execution
// adapter interprets their accepted tool bindings; storage never parses Scope.
func (e *ExecutorCheckpointStore) PendingCheckpointPayloads(ctx context.Context) ([][]byte, error) {
	rows, err := conn(ctx, e.db).QueryContext(ctx, `SELECT payload FROM executor_checkpoints WHERE EXISTS (SELECT 1 FROM runs WHERE runs.session_id=executor_checkpoints.session_id AND runs.state<>'terminal')`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result [][]byte
	for rows.Next() {
		var body []byte
		if err = rows.Scan(&body); err != nil {
			return nil, err
		}
		result = append(result, body)
	}
	return result, rows.Err()
}
