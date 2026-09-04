package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/automation/goal"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
)

// GoalStore is the SQLite persistence adapter for autonomous goals: one row per session, the
// budget and used accumulators JSON blobs read/written whole with the row.
//
// Safe for concurrent use; the *sql.DB serializes writes (MaxOpenConns 1, see
// [Open]).
type GoalStore struct {
	db *sql.DB
}

// NewGoalStore wires a database with the current [Open]-installed schema to the
// autonomous-goal persistence surface.
func NewGoalStore(db *sql.DB) *GoalStore { return &GoalStore{db: db} }

type goalBudgetType string

const (
	goalBudgetUnlimited goalBudgetType = "unlimited"
	goalBudgetLimited   goalBudgetType = "limited"
)

type storedGoalBudget struct {
	Type       goalBudgetType `json:"type"`
	MaxRuns    *int           `json:"max_runs,omitempty"`
	MaxCostUSD *float64       `json:"max_cost_usd,omitempty"`
	MaxSteps   *int           `json:"max_steps,omitempty"`
}

type goalUsed struct {
	Runs    int      `json:"runs"`
	CostUSD *float64 `json:"cost_usd,omitempty"`
	Steps   int      `json:"steps"`
}

// Get returns the Session's explicit optional Goal.
func (g *GoalStore) Get(ctx context.Context, sessionID string) (goal.Current, error) {
	unwritten, err := goal.Unwritten(sessionID)
	if err != nil {
		return goal.Current{}, err
	}
	row := conn(ctx, g.db).QueryRowContext(ctx,
		`SELECT session_id, objective, status, reason_code, reason_detail, provider, model, reasoning_effort, capabilities, budget, used, incarnation_id, revision, created_at, updated_at
		 FROM goals WHERE session_id = ?`, sessionID)
	loaded, err := scanGoal(row)
	if errors.Is(err, sql.ErrNoRows) {
		return unwritten, nil
	}
	if err != nil {
		return goal.Current{}, err
	}
	current, err := goal.CurrentOf(loaded)
	if err != nil {
		return goal.Current{}, fmt.Errorf("sqlite: own Goal: %w", err)
	}
	return current, nil
}

// Save is the Goal CAS. Domain transitions advance revisions before this
// adapter atomically persists the exact replacement.
// INSERT-if-absent (not INSERT OR REPLACE) is deliberate — a stale writer whose
// row was cleared must not resurrect it.
func (g *GoalStore) Save(ctx context.Context, replacement goal.Replacement) (bool, error) {
	if err := replacement.Validate(); err != nil {
		return false, fmt.Errorf("sqlite: validate goal replacement: %w", err)
	}
	record := replacement.State()
	expected := replacement.ExpectedVersion()
	snapshot := record.Snapshot()
	budget, err := encodeGoalBudget(snapshot.Budget)
	if err != nil {
		return false, fmt.Errorf("sqlite: encode goal budget: %w", err)
	}
	used, err := json.Marshal(goalUsed{Runs: snapshot.Used.Runs, CostUSD: snapshot.Used.Cost.OptionalUSD(), Steps: snapshot.Used.Steps})
	if err != nil {
		return false, fmt.Errorf("sqlite: encode goal used: %w", err)
	}
	capabilities, err := encodeRunCapabilities(snapshot.Capabilities)
	if err != nil {
		return false, fmt.Errorf("sqlite: encode goal capabilities: %w", err)
	}
	if expected.IsUnwritten() {
		res, execContextErr := conn(ctx, g.db).ExecContext(ctx,
			`INSERT INTO goals(session_id, objective, status, reason_code, reason_detail, provider, model, reasoning_effort, capabilities, budget, used, incarnation_id, revision, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			 ON CONFLICT(session_id) DO NOTHING`,
			snapshot.SessionID, snapshot.Objective, string(snapshot.Status), string(snapshot.ReasonCode), snapshot.ReasonDetail, snapshot.ModelSelection.Provider(), snapshot.ModelSelection.Model(), snapshot.ModelSelection.ReasoningEffort(),
			capabilities, budget, string(used), snapshot.IncarnationID, snapshot.Revision, snapshot.CreatedAt.UnixNano(), snapshot.UpdatedAt.UnixNano())
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
		`UPDATE goals SET objective = ?, status = ?, reason_code = ?, reason_detail = ?, provider = ?, model = ?, reasoning_effort = ?, capabilities = ?, budget = ?, used = ?, incarnation_id = ?, revision = ?, created_at = ?, updated_at = ?
		 WHERE session_id = ? AND incarnation_id = ? AND revision = ?`,
		snapshot.Objective, string(snapshot.Status), string(snapshot.ReasonCode), snapshot.ReasonDetail, snapshot.ModelSelection.Provider(), snapshot.ModelSelection.Model(), snapshot.ModelSelection.ReasoningEffort(),
		capabilities, budget, string(used), snapshot.IncarnationID, snapshot.Revision, snapshot.CreatedAt.UnixNano(), snapshot.UpdatedAt.UnixNano(),
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

// RecordRun records a terminal goal-owned Run and applies its aggregate
// accounting in one transaction. goal_runs is an immutable idempotency ledger:
// a repeated terminal delivery for the same Run cannot charge the Goal twice,
// while an older incarnation is retained as history but never mutates a newer Goal.
func (g *GoalStore) RecordRun(ctx context.Context, record goal.RunRecord) error {
	if err := record.Validate(); err != nil {
		return fmt.Errorf("sqlite: record Goal Run: %w", err)
	}
	return RunInTx(ctx, g.db, func(ctx context.Context) error {
		var costUSD any
		if value, available := record.Cost.USD(); available {
			costUSD = value
		}
		res, err := conn(ctx, g.db).ExecContext(ctx,
			`INSERT INTO goal_runs(run_id, session_id, incarnation_id, outcome, cost_usd, steps, completed_at)
			 SELECT run_id, session_id, goal_incarnation_id, outcome, ?, steps, finished_at
			   FROM runs
			  WHERE run_id = ? AND session_id = ? AND state = ?
			    AND goal_incarnation_id = ? AND outcome = ?
			    AND steps = ? AND finished_at = ?
			 ON CONFLICT(run_id) DO NOTHING`,
			costUSD,
			record.RunID, record.SessionID, runStateTerminal.databaseValue(),
			record.IncarnationID, record.Outcome.String(), record.Steps,
			record.CompletedAt.UTC().UnixNano())
		if err != nil {
			return fmt.Errorf("sqlite: record Goal Run: %w", err)
		}
		inserted, err := rowsAffected(res)
		if err != nil {
			return err
		}
		if !inserted {
			return g.validateExistingRun(ctx, record)
		}

		current, err := g.Get(ctx, record.SessionID)
		if err != nil {
			return err
		}
		existing, found := current.Goal()
		if !found || existing.IncarnationID() != record.IncarnationID {
			return nil
		}
		expected := existing.Version()
		replacement, err := existing.RecordRun(record)
		if err != nil {
			return fmt.Errorf("sqlite: apply Goal Run: %w", err)
		}
		change, err := goal.NewReplacement(expected, replacement)
		if err != nil {
			return fmt.Errorf("sqlite: prepare goal run replacement: %w", err)
		}
		applied, err := g.Save(ctx, change)
		if err != nil {
			return err
		}
		if !applied {
			return errors.New("sqlite: record Goal Run lost Goal ownership")
		}
		return nil
	})
}

func (g *GoalStore) validateExistingRun(ctx context.Context, record goal.RunRecord) error {
	var (
		sessionID     string
		incarnationID string
		outcome       string
		costUSD       sql.NullFloat64
		steps         int
		completedAt   int64
	)
	err := conn(ctx, g.db).QueryRowContext(ctx,
		`SELECT session_id, incarnation_id, outcome, cost_usd, steps, completed_at
		   FROM goal_runs
		  WHERE run_id = ?`,
		record.RunID,
	).Scan(&sessionID, &incarnationID, &outcome, &costUSD, &steps, &completedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf(
			"%w: Run %q has no matching terminal accounting owner",
			goal.ErrRunIdentityConflict, record.RunID,
		)
	}
	if err != nil {
		return fmt.Errorf("sqlite: inspect existing Goal Run %q: %w", record.RunID, err)
	}
	var costPointer *float64
	if costUSD.Valid {
		costPointer = &costUSD.Float64
	}
	storedCost, err := accounting.CostFromOptional(costPointer)
	if err != nil {
		return fmt.Errorf("sqlite: decode existing Goal Run %q cost: %w", record.RunID, err)
	}
	if sessionID == record.SessionID && incarnationID == record.IncarnationID &&
		outcome == record.Outcome.String() && storedCost.Equal(record.Cost) &&
		steps == record.Steps && completedAt == record.CompletedAt.UTC().UnixNano() {
		return nil
	}
	return fmt.Errorf(
		"%w: Run %q is already bound to a different accounting fact",
		goal.ErrRunIdentityConflict,
		record.RunID,
	)
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
	rows, err := conn(ctx, g.db).QueryContext(ctx,
		`SELECT session_id, objective, status, reason_code, reason_detail, provider, model, reasoning_effort, capabilities, budget, used, incarnation_id, revision, created_at, updated_at FROM goals`)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list goals: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []goal.Goal
	for rows.Next() {
		loaded, err := scanGoal(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, loaded)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: list goals: %w", err)
	}
	return out, nil
}

// scanGoal decodes one row of the goals table. Both queries select the same
// fourteen columns in the same order (session_id first), so [scanRow] covers
// *sql.Row (Get) and *sql.Rows (List) alike.
func scanGoal(row scanRow) (goal.Goal, error) {
	var (
		sessionID, objective, incarnationID string
		revision                            int64
		status                              string
		reasonCode                          string
		reasonDetail                        string
		provider, model, reasoningEffort    string
		capabilitiesJSON                    string
		budgetJSON, usedJSON                string
		createdAt, updatedAt                int64
	)
	if err := row.Scan(&sessionID, &objective, &status, &reasonCode, &reasonDetail, &provider, &model, &reasoningEffort, &capabilitiesJSON, &budgetJSON, &usedJSON, &incarnationID, &revision, &createdAt, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return goal.Goal{}, err
		}
		return goal.Goal{}, fmt.Errorf("sqlite: scan goal: %w", err)
	}
	selection, err := modelref.NewWithReasoningEffort(provider, model, reasoningEffort)
	if err != nil {
		return goal.Goal{}, fmt.Errorf("sqlite: decode goal model selection: %w", err)
	}
	capabilities, err := decodeRunCapabilities(capabilitiesJSON)
	if err != nil {
		return goal.Goal{}, fmt.Errorf("sqlite: decode goal capabilities: %w", err)
	}
	budget, err := decodeGoalBudget(budgetJSON)
	if err != nil {
		return goal.Goal{}, fmt.Errorf("sqlite: decode goal budget: %w", err)
	}
	var used goalUsed
	if err := json.Unmarshal([]byte(usedJSON), &used); err != nil {
		return goal.Goal{}, fmt.Errorf("sqlite: decode goal used: %w", err)
	}
	usedCost, err := accounting.CostFromOptional(used.CostUSD)
	if err != nil {
		return goal.Goal{}, fmt.Errorf("sqlite: decode goal used cost: %w", err)
	}
	value, err := goal.Restore(goal.Snapshot{
		SessionID: sessionID, Objective: objective, Status: goal.Status(status),
		ReasonCode: goal.ReasonCode(reasonCode), ReasonDetail: reasonDetail,
		ModelSelection: selection, Capabilities: capabilities,
		Budget:        budget,
		Used:          goal.Usage{Runs: used.Runs, Cost: usedCost, Steps: used.Steps},
		IncarnationID: incarnationID, Revision: revision,
		CreatedAt: time.Unix(0, createdAt).UTC(), UpdatedAt: time.Unix(0, updatedAt).UTC(),
	})
	if err != nil {
		return goal.Goal{}, fmt.Errorf("sqlite: validate goal: %w", err)
	}
	return value, nil
}

func encodeGoalBudget(budget goal.Budget) (string, error) {
	if err := budget.Validate(); err != nil {
		return "", err
	}
	row := storedGoalBudget{Type: goalBudgetUnlimited}
	if !budget.Unlimited() {
		row.Type = goalBudgetLimited
		if value, limited := budget.MaxRuns(); limited {
			row.MaxRuns = &value
		}
		if value, limited := budget.MaxCostUSD(); limited {
			row.MaxCostUSD = &value
		}
		if value, limited := budget.MaxSteps(); limited {
			row.MaxSteps = &value
		}
	}
	encoded, err := json.Marshal(row)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func decodeGoalBudget(encoded string) (goal.Budget, error) {
	var row storedGoalBudget
	decoder := json.NewDecoder(strings.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&row); err != nil {
		return goal.Budget{}, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("multiple JSON values")
		}
		return goal.Budget{}, err
	}
	switch row.Type {
	case goalBudgetUnlimited:
		if row.MaxRuns != nil || row.MaxCostUSD != nil || row.MaxSteps != nil {
			return goal.Budget{}, errors.New("unlimited budget carries a limit")
		}
		return goal.UnlimitedBudget(), nil
	case goalBudgetLimited:
		return goal.NewBudget(goal.BudgetLimits{
			MaxRuns: row.MaxRuns, MaxCostUSD: row.MaxCostUSD, MaxSteps: row.MaxSteps,
		})
	default:
		return goal.Budget{}, fmt.Errorf("unknown budget type %q", row.Type)
	}
}
