package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
)

// ModeStore records the Runtime's permission modes: the default a user chose
// and which Sessions are in Plan mode. A Session without a row follows the
// Runtime default, so leaving Plan mode removes the row instead of pinning a
// copy of that default to the Session.
type ModeStore struct{ db *sql.DB }

func NewModeStore(db *sql.DB) *ModeStore {
	return &ModeStore{db: db}
}

// approvalDefaultModeSchema holds the one Runtime default a user chose. Its
// values are the domain's default vocabulary; Plan is session-only.
func approvalDefaultModeSchema() string {
	return fmt.Sprintf(`CREATE TABLE IF NOT EXISTS approval_default_mode (
			id   INTEGER PRIMARY KEY CHECK (id = 1),
			mode TEXT NOT NULL CHECK (mode IN ('%s', '%s', '%s'))
		)`, approval.ModeSafe, approval.ModeBalanced, approval.ModeYolo)
}

// DefaultMode returns the chosen Runtime default; found is false until one is
// chosen.
func (p *ModeStore) DefaultMode(ctx context.Context) (approval.Mode, bool, error) {
	var stored string
	err := conn(ctx, p.db).QueryRowContext(ctx,
		`SELECT mode FROM approval_default_mode WHERE id = 1`,
	).Scan(&stored)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("sqlite: read default approval mode: %w", err)
	}
	mode := approval.Mode(stored)
	if !mode.ValidDefault() {
		return "", false, fmt.Errorf("sqlite: decode default approval mode: %w: %q", approval.ErrInvalidMode, stored)
	}
	return mode, true, nil
}

// SetDefaultMode records the Runtime default a user chose.
func (p *ModeStore) SetDefaultMode(ctx context.Context, mode approval.Mode) error {
	if _, err := conn(ctx, p.db).ExecContext(ctx,
		`INSERT INTO approval_default_mode(id, mode) VALUES (1, ?) ON CONFLICT(id) DO UPDATE SET mode = excluded.mode`,
		string(mode),
	); err != nil {
		return fmt.Errorf("sqlite: save default approval mode: %w", err)
	}
	return nil
}

func (p *ModeStore) PlanModeActive(ctx context.Context, sessionID string) (bool, error) {
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
func (p *ModeStore) StartPlanMode(ctx context.Context, sessionID string) (bool, error) {
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
func (p *ModeStore) EndPlanMode(ctx context.Context, sessionID string) (bool, error) {
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
func (p *ModeStore) DeleteSession(ctx context.Context, sessionID string) error {
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
