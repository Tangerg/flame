package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// PlanModeStore records which Sessions are in Plan mode. A Session without a
// row follows the Runtime default approval mode, so leaving Plan mode removes
// the row instead of pinning a copy of that default to the Session.
type PlanModeStore struct{ db *sql.DB }

func NewPlanModeStore(db *sql.DB) *PlanModeStore {
	return &PlanModeStore{db: db}
}

func (p *PlanModeStore) PlanModeActive(ctx context.Context, sessionID string) (bool, error) {
	if err := validateSessionResource("read Session Plan mode", sessionID); err != nil {
		return false, err
	}
	var present int
	err := conn(ctx, p.db).QueryRowContext(ctx,
		`SELECT 1 FROM session_plan_modes WHERE session_id = ?`, sessionID,
	).Scan(&present)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("sqlite: read session Plan mode: %w", err)
	}
	return true, nil
}

// StartPlanMode reports whether the Session entered Plan mode now.
func (p *PlanModeStore) StartPlanMode(ctx context.Context, sessionID string) (bool, error) {
	if err := validateSessionResource("enter Session Plan mode", sessionID); err != nil {
		return false, err
	}
	result, err := conn(ctx, p.db).ExecContext(ctx,
		`INSERT INTO session_plan_modes(session_id) VALUES (?) ON CONFLICT(session_id) DO NOTHING`, sessionID,
	)
	if err != nil {
		return false, fmt.Errorf("sqlite: enter session Plan mode: %w", err)
	}
	return rowChanged(result, "enter session Plan mode")
}

// EndPlanMode reports whether the Session left Plan mode now.
func (p *PlanModeStore) EndPlanMode(ctx context.Context, sessionID string) (bool, error) {
	if err := validateSessionResource("leave Session Plan mode", sessionID); err != nil {
		return false, err
	}
	result, err := conn(ctx, p.db).ExecContext(ctx,
		`DELETE FROM session_plan_modes WHERE session_id = ?`, sessionID,
	)
	if err != nil {
		return false, fmt.Errorf("sqlite: leave session Plan mode: %w", err)
	}
	return rowChanged(result, "leave session Plan mode")
}

// DeleteSession removes the Plan mode owned by one Session. Session restore
// uses this before reseeding portable material: permission policy is local
// Runtime state and is deliberately not inherited from the Session that an
// imported archive replaces.
func (p *PlanModeStore) DeleteSession(ctx context.Context, sessionID string) error {
	_, err := p.EndPlanMode(ctx, sessionID)
	return err
}

func rowChanged(result sql.Result, operation string) (bool, error) {
	changed, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("sqlite: %s: %w", operation, err)
	}
	return changed == 1, nil
}
