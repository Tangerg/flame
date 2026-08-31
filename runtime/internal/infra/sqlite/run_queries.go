package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	rundomain "github.com/Tangerg/flame/runtime/internal/domain/run"
)

// runColumns is the whole Run row, joined with the open interrupts a parked Run
// is waiting on — kept in the interrupts table so one park is one record.
const runColumns = `r.run_id, r.session_id, r.spawned_by_item_id, r.parent_run_id, r.root_run_id,
	r.state, r.active_segment_id, r.outcome,
	r.provider, r.model, r.reasoning_effort, r.goal_incarnation_id, r.detail,
	r.steps, r.active_duration_ns, r.usage, r.context_tokens, r.problem,
	r.max_total_tokens, r.max_steps, r.max_budget_usd, r.capabilities, tree_root.capabilities,
	r.message_mark, r.started_at, r.finished_at, r.updated_at, i.payload`

// runReadJoins materializes the root-owned capabilities and pending set for
// every Run in the tree. scanRun filters the aggregate payload by source Run ID,
// so a suspended sibling reads an empty direct-interrupt list rather than
// claiming another Run's questions.
const runReadJoins = `LEFT JOIN runs AS tree_root
		   ON tree_root.run_id = r.root_run_id AND tree_root.session_id = r.session_id
		 LEFT JOIN interrupts AS i
		   ON i.root_run_id = CASE
		        WHEN r.root_run_id = '' THEN r.run_id
		        ELSE r.root_run_id
		      END
		  AND i.session_id = r.session_id`

// PageRuns returns one page of Runs a caller may browse, newest admission first,
// scoped to sessionID and statuses when provided. Descendants are excluded unless
// includeDescendants is true.
func (r *RunStore) PageRuns(ctx context.Context, sessionID string, statuses []rundomain.Status, includeDescendants bool, beforeStartedAt int64, beforeRunID string, limit int) ([]rundomain.Run, error) {
	if err := validateOptionalSessionResource("page Runs", sessionID); err != nil {
		return nil, err
	}
	if err := validateOptionalRunResource("page Runs anchor", beforeRunID); err != nil {
		return nil, err
	}
	query := `SELECT ` + runColumns + `
		 FROM runs AS r
		 ` + runReadJoins
	var args []any
	var conditions []string
	if !includeDescendants {
		conditions = append(conditions, `r.root_run_id = ''`)
	}
	if sessionID != "" {
		conditions = append(conditions, `r.session_id = ?`)
		args = append(args, sessionID)
	}
	if len(statuses) > 0 {
		columns, err := stateColumns(statuses)
		if err != nil {
			return nil, fmt.Errorf("sqlite: page runs: %w", err)
		}
		conditions = append(conditions, `r.state IN (`+placeholders(len(columns))+`)`)
		args = append(args, columns...)
	}
	if beforeRunID != "" {
		conditions = append(conditions, `(r.started_at < ? OR (r.started_at = ? AND r.run_id < ?))`)
		args = append(args, beforeStartedAt, beforeStartedAt, beforeRunID)
	}
	if len(conditions) > 0 {
		query += ` WHERE ` + strings.Join(conditions, ` AND `)
	}
	query += ` ORDER BY r.started_at DESC, r.run_id DESC`
	if limit > 0 {
		query += ` LIMIT ?`
		args = append(args, limit)
	}

	rows, err := conn(ctx, r.db).QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite: page runs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []rundomain.Run
	for rows.Next() {
		run, err := scanRun(rows)
		if err != nil {
			return nil, fmt.Errorf("sqlite: page runs: %w", err)
		}
		out = append(out, run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: page runs: %w", err)
	}
	return out, nil
}

// Run returns one Run by id alone, whatever state it is in.
func (r *RunStore) Run(ctx context.Context, runID string) (rundomain.Run, bool, error) {
	if err := validateRunResource("read Run", runID); err != nil {
		return rundomain.Run{}, false, err
	}
	row := conn(ctx, r.db).QueryRowContext(ctx,
		`SELECT `+runColumns+`
		 FROM runs AS r
		 `+runReadJoins+`
		 WHERE r.run_id = ?`, runID)
	run, err := scanRun(row)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return rundomain.Run{}, false, nil
	case err != nil:
		return rundomain.Run{}, false, fmt.Errorf("sqlite: read run %q: %w", runID, err)
	}
	return run, true, nil
}

// Tree resolves runID to its tree root and returns that root plus every
// descendant in one SQLite read. Application code derives canonical order.
func (r *RunStore) Tree(ctx context.Context, runID string) ([]rundomain.Run, error) {
	if err := validateRunResource("read Run tree", runID); err != nil {
		return nil, err
	}
	rows, err := conn(ctx, r.db).QueryContext(ctx,
		`WITH target AS (
		    SELECT CASE WHEN root_run_id = '' THEN run_id ELSE root_run_id END AS tree_root_id
		      FROM runs
		     WHERE run_id = ?
		 )
		 SELECT `+runColumns+`
		 FROM runs AS r
		 CROSS JOIN target
		 `+runReadJoins+`
		 WHERE r.run_id = target.tree_root_id OR r.root_run_id = target.tree_root_id`,
		runID,
	)
	if err != nil {
		return nil, fmt.Errorf("sqlite: read tree containing run %q: %w", runID, err)
	}
	defer func() { _ = rows.Close() }()

	var runs []rundomain.Run
	for rows.Next() {
		run, err := scanRun(rows)
		if err != nil {
			return nil, fmt.Errorf("sqlite: read tree containing run %q: %w", runID, err)
		}
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: read tree containing run %q: %w", runID, err)
	}
	return runs, nil
}

// RunsWithAncestors returns the named Runs and the ancestors connecting them
// to their roots without loading unrelated Runs from the Session.
func (r *RunStore) RunsWithAncestors(ctx context.Context, runIDs []string) ([]rundomain.Run, error) {
	if len(runIDs) == 0 {
		return nil, nil
	}
	args := make([]any, 0, len(runIDs))
	for _, id := range runIDs {
		if err := validateRunResource("read Runs with ancestors", id); err != nil {
			return nil, err
		}
		args = append(args, id)
	}
	rows, err := conn(ctx, r.db).QueryContext(ctx,
		`WITH RECURSIVE lineage(run_id, parent_run_id) AS (
			SELECT run_id, parent_run_id FROM runs
			 WHERE run_id IN (`+placeholders(len(runIDs))+`)
			UNION
			SELECT parent.run_id, parent.parent_run_id
			  FROM runs AS parent
			  JOIN lineage AS child ON child.parent_run_id = parent.run_id
		)
		 SELECT `+runColumns+`
		 FROM runs AS r
		 `+runReadJoins+`
		 WHERE r.run_id IN (SELECT run_id FROM lineage)
		 ORDER BY r.started_at DESC, r.run_id DESC`, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite: read runs with ancestors: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []rundomain.Run
	for rows.Next() {
		run, err := scanRun(rows)
		if err != nil {
			return nil, fmt.Errorf("sqlite: read runs with ancestors: %w", err)
		}
		out = append(out, run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: read runs with ancestors: %w", err)
	}
	return out, nil
}

// ListRuns returns a session's complete Run aggregates in admission order.
func (r *RunStore) ListRuns(ctx context.Context, sessionID string) ([]rundomain.Run, error) {
	if err := validateSessionResource("list Session Runs", sessionID); err != nil {
		return nil, err
	}
	rows, err := conn(ctx, r.db).QueryContext(ctx,
		`SELECT `+runColumns+`
		 FROM runs AS r
		 `+runReadJoins+`
		 WHERE r.session_id = ? ORDER BY r.started_at, r.run_id`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list runs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []rundomain.Run
	for rows.Next() {
		run, err := scanRun(rows)
		if err != nil {
			return nil, fmt.Errorf("sqlite: list runs: %w", err)
		}
		out = append(out, run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: list runs: %w", err)
	}
	return out, nil
}

func stateColumns(statuses []rundomain.Status) ([]any, error) {
	out := make([]any, 0, len(statuses))
	for _, status := range statuses {
		if !status.Valid() {
			return nil, fmt.Errorf("unknown run status %q", status)
		}
		out = append(out, stateColumn(status).databaseValue())
	}
	return out, nil
}

func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?, ", n), ", ")
}
