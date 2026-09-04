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

	"github.com/Tangerg/flame/runtime/internal/domain/session/plan"
)

// PlanStore persists one complete ordered Plan per session. Plans are replaced
// wholesale, so one JSON value and one monotonic revision are the entire latest
// projection; Run boundaries retain the historical values needed by fork and
// rollback.
type PlanStore struct{ db *sql.DB }

type planStepRow struct {
	Description string      `json:"description"`
	Status      plan.Status `json:"status"`
}

func NewPlanStore(db *sql.DB) *PlanStore { return &PlanStore{db: db} }

func (p *PlanStore) List(ctx context.Context, sessionID string) ([]plan.Step, error) {
	state, err := p.State(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	return state.Steps(), nil
}

// State returns an explicit unwritten Current when no Plan row exists.
func (p *PlanStore) State(ctx context.Context, sessionID string) (plan.Current, error) {
	if err := validateSessionResource("read Session Plan", sessionID); err != nil {
		return plan.Current{}, err
	}
	var (
		stepsJSON string
		revision  uint64
		updatedNs int64
	)
	err := conn(ctx, p.db).QueryRowContext(ctx,
		`SELECT steps, revision, updated_at FROM session_plans WHERE session_id = ?`, sessionID,
	).Scan(&stepsJSON, &revision, &updatedNs)
	if errors.Is(err, sql.ErrNoRows) {
		return plan.Current{}, nil
	}
	if err != nil {
		return plan.Current{}, fmt.Errorf("sqlite: read Plan: %w", err)
	}
	steps, err := decodePlanSteps(stepsJSON)
	if err != nil {
		return plan.Current{}, err
	}
	state, err := plan.Restore(plan.Snapshot{
		Steps: steps, Revision: revision, UpdatedAt: time.Unix(0, updatedNs).UTC(),
	})
	if err != nil {
		return plan.Current{}, fmt.Errorf("sqlite: restore Plan: %w", err)
	}
	current, err := plan.CurrentOf(state)
	if err != nil {
		return plan.Current{}, fmt.Errorf("sqlite: own Plan state: %w", err)
	}
	return current, nil
}

func decodePlanSteps(stepsJSON string) ([]plan.Step, error) {
	if stepsJSON == "" {
		return nil, nil
	}
	var rows []planStepRow
	decoder := json.NewDecoder(strings.NewReader(stepsJSON))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&rows); err != nil {
		return nil, fmt.Errorf("sqlite: decode Plan: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("multiple JSON values")
		}
		return nil, fmt.Errorf("sqlite: decode Plan: %w", err)
	}
	steps := make([]plan.Step, len(rows))
	for index, row := range rows {
		steps[index] = plan.Step{Description: row.Description, Status: row.Status}
	}
	if err := plan.ValidateSteps(steps); err != nil {
		return nil, fmt.Errorf("sqlite: validate Plan: %w", err)
	}
	return steps, nil
}

// Save persists one Domain-decided replacement iff its expected version is
// still current. It assigns neither time nor revision.
func (p *PlanStore) Save(ctx context.Context, sessionID string, change plan.Replacement) error {
	if err := validateSessionResource("save Session Plan", sessionID); err != nil {
		return err
	}
	if err := change.Validate(); err != nil {
		return fmt.Errorf("sqlite: validate Plan replacement: %w", err)
	}
	expected := change.ExpectedVersion()
	replacement := change.State()
	steps := replacement.Steps()
	rows := make([]planStepRow, len(steps))
	for index, step := range steps {
		rows[index] = planStepRow{Description: step.Description, Status: step.Status}
	}
	data, err := json.Marshal(rows)
	if err != nil {
		return fmt.Errorf("sqlite: encode Plan: %w", err)
	}
	var result sql.Result
	if expected.IsUnwritten() {
		result, err = conn(ctx, p.db).ExecContext(ctx,
			`INSERT INTO session_plans(session_id, steps, revision, updated_at)
			 VALUES (?, ?, ?, ?) ON CONFLICT(session_id) DO NOTHING`,
			sessionID, string(data), replacement.Revision(), replacement.UpdatedAt().UnixNano(),
		)
	} else {
		expectedRevision, committed := expected.Revision()
		if !committed {
			return fmt.Errorf("sqlite: Plan expected version lost committed revision: %w", plan.ErrInvalid)
		}
		result, err = conn(ctx, p.db).ExecContext(ctx,
			`UPDATE session_plans SET steps = ?, revision = ?, updated_at = ?
			 WHERE session_id = ? AND revision = ?`,
			string(data), replacement.Revision(), replacement.UpdatedAt().UnixNano(), sessionID, expectedRevision,
		)
	}
	if err != nil {
		return fmt.Errorf("sqlite: save Plan: %w", err)
	}
	written, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite: inspect Plan save: %w", err)
	}
	if written != 1 {
		return fmt.Errorf("sqlite: expected Plan %s: %w", expected, plan.ErrRevisionConflict)
	}
	return nil
}

// CaptureBoundary freezes the session's current Plan at one terminal Run.
func (p *PlanStore) CaptureBoundary(ctx context.Context, sessionID, runID string) error {
	if err := validateSessionResource("capture Plan boundary", sessionID); err != nil {
		return err
	}
	if err := validateRunResource("capture Plan boundary", runID); err != nil {
		return err
	}
	if _, err := conn(ctx, p.db).ExecContext(ctx,
		`INSERT INTO plan_boundaries(run_id, steps)
		 VALUES (?, COALESCE((SELECT steps FROM session_plans WHERE session_id = ?), '[]'))`,
		runID, sessionID,
	); err != nil {
		return fmt.Errorf("sqlite: capture Plan boundary for Run %q: %w", runID, err)
	}
	return nil
}

// Boundary returns the Plan captured by runID. recorded=false means the Run
// never captured a boundary; it does not mean an empty Plan.
func (p *PlanStore) Boundary(ctx context.Context, runID string) ([]plan.Step, bool, error) {
	if err := validateRunResource("read Plan boundary", runID); err != nil {
		return nil, false, err
	}
	var stepsJSON string
	err := conn(ctx, p.db).QueryRowContext(ctx,
		`SELECT steps FROM plan_boundaries WHERE run_id = ?`, runID,
	).Scan(&stepsJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("sqlite: read Plan boundary for Run %q: %w", runID, err)
	}
	steps, err := decodePlanSteps(stepsJSON)
	if err != nil {
		return nil, false, err
	}
	return steps, true, nil
}

func (p *PlanStore) DeleteSession(ctx context.Context, sessionID string) error {
	if err := validateSessionResource("delete Session Plan", sessionID); err != nil {
		return err
	}
	if _, err := conn(ctx, p.db).ExecContext(ctx,
		`DELETE FROM session_plans WHERE session_id = ?`, sessionID,
	); err != nil {
		return fmt.Errorf("sqlite: delete session Plan: %w", err)
	}
	return nil
}
