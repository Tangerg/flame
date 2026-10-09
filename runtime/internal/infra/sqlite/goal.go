package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/automation/goal"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
)

// GoalStore is the SQLite persistence adapter for autonomous goals: one row per
// session. A Goal's usage is not stored; it is folded from the incarnation's
// terminal Runs on every read.
//
// Safe for concurrent use; the *sql.DB serializes writes (MaxOpenConns 1, see
// [Open]).
type GoalStore struct {
	db *sql.DB
}

// NewGoalStore wires a database with the current [Open]-installed schema to the
// autonomous-goal persistence surface.
func NewGoalStore(db *sql.DB) *GoalStore { return &GoalStore{db: db} }

// Get reads the Goal and its Run-owned usage from one storage snapshot.
func (g *GoalStore) Get(ctx context.Context, sessionID string) (goal.Current, error) {
	unwritten, err := goal.Unwritten(sessionID)
	if err != nil {
		return goal.Current{}, err
	}
	current := unwritten
	err = RunInTx(ctx, g.db, func(ctx context.Context) error {
		row := conn(ctx, g.db).QueryRowContext(ctx,
			`SELECT session_id, objective, status, reason_code, reason_detail, provider, model, reasoning_effort, capabilities, incarnation_id, revision, created_at, updated_at
			 FROM goals WHERE session_id = ?`, sessionID)
		snapshot, err := scanGoal(row)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		loaded, err := g.restore(ctx, snapshot)
		if err != nil {
			return err
		}
		current, err = goal.CurrentOf(loaded)
		if err != nil {
			return fmt.Errorf("sqlite: own Goal: %w", err)
		}
		return nil
	})
	if err != nil {
		return goal.Current{}, err
	}
	return current, nil
}

// Save is the Goal CAS. Domain transitions advance revisions before this
// adapter atomically persists the exact replacement.
// INSERT-if-absent (not INSERT OR REPLACE) is deliberate — a stale writer whose
// row was cleared must not resurrect it.
func (g *GoalStore) Save(ctx context.Context, replacement goal.Replacement) (bool, error) {
	record := replacement.State()
	expected := replacement.ExpectedVersion()
	snapshot := record.Snapshot()
	capabilities, err := encodeRunCapabilities(snapshot.Capabilities)
	if err != nil {
		return false, fmt.Errorf("sqlite: encode goal capabilities: %w", err)
	}
	if expected.IsUnwritten() {
		res, execContextErr := conn(ctx, g.db).ExecContext(ctx,
			`INSERT INTO goals(session_id, objective, status, reason_code, reason_detail, provider, model, reasoning_effort, capabilities, incarnation_id, revision, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			 ON CONFLICT(session_id) DO NOTHING`,
			snapshot.SessionID, snapshot.Objective, string(snapshot.Status), string(snapshot.ReasonCode), snapshot.ReasonDetail, snapshot.ModelSelection.Provider(), snapshot.ModelSelection.Model(), snapshot.ModelSelection.ReasoningEffort(),
			capabilities, snapshot.IncarnationID, snapshot.Revision, snapshot.CreatedAt.UnixNano(), snapshot.UpdatedAt.UnixNano())
		if execContextErr != nil {
			return false, fmt.Errorf("sqlite: insert goal: %w", execContextErr)
		}
		applied, execContextErr := rowsAffected(res)
		if execContextErr != nil || !applied {
			return applied, execContextErr
		}
		return true, nil
	}
	expectedIncarnation, committed := expected.IncarnationID()
	expectedRevision, revisionCommitted := expected.Revision()
	if !committed || !revisionCommitted {
		return false, errors.New("sqlite: committed Goal version lost identity")
	}
	res, err := conn(ctx, g.db).ExecContext(ctx,
		`UPDATE goals SET objective = ?, status = ?, reason_code = ?, reason_detail = ?, provider = ?, model = ?, reasoning_effort = ?, capabilities = ?, incarnation_id = ?, revision = ?, created_at = ?, updated_at = ?
		 WHERE session_id = ? AND incarnation_id = ? AND revision = ?`,
		snapshot.Objective, string(snapshot.Status), string(snapshot.ReasonCode), snapshot.ReasonDetail, snapshot.ModelSelection.Provider(), snapshot.ModelSelection.Model(), snapshot.ModelSelection.ReasoningEffort(),
		capabilities, snapshot.IncarnationID, snapshot.Revision, snapshot.CreatedAt.UnixNano(), snapshot.UpdatedAt.UnixNano(),
		snapshot.SessionID, expectedIncarnation, expectedRevision)
	if err != nil {
		return false, fmt.Errorf("sqlite: save goal: %w", err)
	}
	applied, err := rowsAffected(res)
	if err != nil || !applied {
		return applied, err
	}
	return true, nil
}

func rowsAffected(res sql.Result) (bool, error) {
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("sqlite: goal rows affected: %w", err)
	}
	return n == 1, nil
}

// Clear removes the session's goal unconditionally; a missing goal is not an
// error.
func (g *GoalStore) Clear(ctx context.Context, sessionID string) error {
	if err := validateSessionResource("clear Goal", sessionID); err != nil {
		return err
	}
	if _, err := conn(ctx, g.db).ExecContext(ctx, `DELETE FROM goals WHERE session_id = ?`, sessionID); err != nil {
		return fmt.Errorf("sqlite: clear goal: %w", err)
	}
	return nil
}

// ClearIf removes the session's goal only when its version matches expected
// (the loop's CAS delete), reporting whether it applied.
func (g *GoalStore) ClearIf(ctx context.Context, sessionID string, expected goal.Version) (bool, error) {
	if err := validateSessionResource("clear Goal with version", sessionID); err != nil {
		return false, err
	}
	incarnationID, committed := expected.IncarnationID()
	revision, revisionCommitted := expected.Revision()
	if err := expected.Validate(); err != nil || !committed || !revisionCommitted || expected.SessionID() != sessionID {
		return false, fmt.Errorf("sqlite: clear Goal with invalid version: %w", errors.Join(err, goal.ErrInvalid))
	}
	res, err := conn(ctx, g.db).ExecContext(ctx,
		`DELETE FROM goals WHERE session_id = ? AND incarnation_id = ? AND revision = ?`, sessionID, incarnationID, revision)
	if err != nil {
		return false, fmt.Errorf("sqlite: clear goal (cas): %w", err)
	}
	return rowsAffected(res)
}

// List returns every stored goal (for the boot reconcile).
func (g *GoalStore) List(ctx context.Context) ([]goal.Goal, error) {
	var out []goal.Goal
	err := RunInTx(ctx, g.db, func(ctx context.Context) error {
		rows, err := conn(ctx, g.db).QueryContext(ctx,
			`SELECT session_id, objective, status, reason_code, reason_detail, provider, model, reasoning_effort, capabilities, incarnation_id, revision, created_at, updated_at FROM goals`)
		if err != nil {
			return fmt.Errorf("sqlite: list goals: %w", err)
		}
		var snapshots []goal.Snapshot
		for rows.Next() {
			snapshot, err := scanGoal(rows)
			if err != nil {
				_ = rows.Close()
				return err
			}
			snapshots = append(snapshots, snapshot)
		}
		if err := errors.Join(rows.Err(), rows.Close()); err != nil {
			return fmt.Errorf("sqlite: list goals: %w", err)
		}
		out = make([]goal.Goal, 0, len(snapshots))
		for _, snapshot := range snapshots {
			loaded, err := g.restore(ctx, snapshot)
			if err != nil {
				return err
			}
			out = append(out, loaded)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// scanGoal decodes one row of the goals table. Both queries select the same
// thirteen columns in the same order (session_id first), so [scanRow] covers
// *sql.Row (Get) and *sql.Rows (List) alike. The returned snapshot has no
// usage yet; [GoalStore.restore] folds it from the Runs.
func scanGoal(row scanRow) (goal.Snapshot, error) {
	var (
		sessionID, objective, incarnationID string
		revision                            int64
		status                              string
		reasonCode                          string
		reasonDetail                        string
		provider, model, reasoningEffort    string
		capabilitiesJSON                    string
		createdAt, updatedAt                int64
	)
	if err := row.Scan(&sessionID, &objective, &status, &reasonCode, &reasonDetail, &provider, &model, &reasoningEffort, &capabilitiesJSON, &incarnationID, &revision, &createdAt, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return goal.Snapshot{}, err
		}
		return goal.Snapshot{}, fmt.Errorf("sqlite: scan goal: %w", err)
	}
	selection, err := modelref.NewWithReasoningEffort(provider, model, reasoningEffort)
	if err != nil {
		return goal.Snapshot{}, fmt.Errorf("sqlite: decode goal model selection: %w", err)
	}
	capabilities, err := decodeRunCapabilities(capabilitiesJSON)
	if err != nil {
		return goal.Snapshot{}, fmt.Errorf("sqlite: decode goal capabilities: %w", err)
	}
	return goal.Snapshot{
		SessionID: sessionID, Objective: objective, Status: goal.Status(status),
		ReasonCode: goal.ReasonCode(reasonCode), ReasonDetail: reasonDetail,
		ModelSelection: selection, Capabilities: capabilities,
		IncarnationID: incarnationID, Revision: revision,
		CreatedAt: time.Unix(0, createdAt).UTC(), UpdatedAt: time.Unix(0, updatedAt).UTC(),
	}, nil
}

func (g *GoalStore) restore(ctx context.Context, snapshot goal.Snapshot) (goal.Goal, error) {
	rows, err := conn(ctx, g.db).QueryContext(ctx,
		`SELECT `+runColumns+` FROM runs AS r `+runReadJoins+`
		  WHERE r.session_id = ? AND r.goal_incarnation_id = ? AND r.state = ?`,
		snapshot.SessionID, snapshot.IncarnationID, runStateTerminal.databaseValue())
	if err != nil {
		return goal.Goal{}, fmt.Errorf("sqlite: read goal Runs: %w", err)
	}
	var charged []run.Run
	for rows.Next() {
		value, err := scanRun(rows)
		if err != nil {
			_ = rows.Close()
			return goal.Goal{}, fmt.Errorf("sqlite: scan goal Run: %w", err)
		}
		charged = append(charged, value)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return goal.Goal{}, fmt.Errorf("sqlite: read goal Runs: %w", err)
	}
	snapshot.Used, err = goal.UsageOf(snapshot.IncarnationID, charged)
	if err != nil {
		return goal.Goal{}, fmt.Errorf("sqlite: fold goal usage: %w", err)
	}
	value, err := goal.Restore(snapshot)
	if err != nil {
		return goal.Goal{}, fmt.Errorf("sqlite: validate goal: %w", err)
	}
	return value, nil
}
