package persistence

import (
	"context"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
)

func (r *RunStore) ListNonTerminalRuns(ctx context.Context) ([]run.Run, error) {
	return scopedValue(ctx, r.db.read, func(ctx context.Context) ([]run.Run, error) {
		rows, err := r.db.connection(ctx).QueryContext(ctx, `SELECT `+runColumns+`
		   FROM runs AS r
		   `+runReadJoins+`
		  WHERE r.state != ?
		  ORDER BY r.created_at, r.run_id`, string(run.Finished))
		if err != nil {
			return nil, fmt.Errorf("sqlite: list non-terminal Runs: %w", err)
		}
		defer rows.Close()
		var runs []run.Run
		for rows.Next() {
			run, err := scanRunForRecovery(rows)
			if err != nil {
				return nil, fmt.Errorf("sqlite: list non-terminal Runs: %w", err)
			}
			runs = append(runs, run)
		}
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("sqlite: list non-terminal Runs: %w", err)
		}
		return runs, nil
	})
}
