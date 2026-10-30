package persistence

import (
	"context"
	"fmt"

	"github.com/Tangerg/flame/qimu/sqlite"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
)

// ListNonTerminalRuns returns the complete unique set of constructed open Runs.
// The primary key and active-root index own identity and Session uniqueness;
// Domain restoration owns individual records, while recovery owns tree survival.
func (r *RunStore) ListNonTerminalRuns(ctx context.Context) ([]run.Run, error) {
	rows, err := sqlite.Connection(ctx, r.db).QueryContext(ctx,
		`SELECT `+runColumns+`
		   FROM runs AS r
		   `+runReadJoins+`
		  WHERE r.state != ?
		  ORDER BY r.created_at, r.run_id`,
		runStateTerminal.databaseValue())
	if err != nil {
		return nil, fmt.Errorf("sqlite: list non-terminal Runs: %w", err)
	}
	defer func() { _ = rows.Close() }()

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
}
