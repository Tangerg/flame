package persistence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
)

// Leaving Plan mode removes the row instead of pinning a copy of the Runtime
// default to the Session.
type ModeStore struct{ db *database }

func newModeStore(db *database) *ModeStore {
	return &ModeStore{db: db}
}

func approvalDefaultModeSchema() string {
	return fmt.Sprintf(`CREATE TABLE IF NOT EXISTS approval_default_mode (
			id   INTEGER PRIMARY KEY CHECK (id = 1),
			mode TEXT NOT NULL CHECK (mode IN ('%s', '%s', '%s'))
		)`, approval.ModeSafe, approval.ModeBalanced, approval.ModeYolo)
}

func (p *ModeStore) DefaultMode(ctx context.Context) (approval.Mode, bool, error) {
	return scopedPair(ctx, p.db.read, func(ctx context.Context) (approval.Mode, bool, error) {
		var stored string
		err := p.db.connection(ctx).QueryRowContext(ctx, `SELECT mode FROM approval_default_mode WHERE id = 1`).Scan(&stored)
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
	})
}

func (p *ModeStore) SetDefaultMode(ctx context.Context, mode approval.Mode) error {
	return p.db.write(ctx, func(ctx context.Context) error {
		if _, err := p.db.connection(ctx).ExecContext(ctx, `INSERT INTO approval_default_mode(id, mode) VALUES (1, ?) ON CONFLICT(id) DO UPDATE SET mode = excluded.mode`, string(mode)); err != nil {
			return fmt.Errorf("sqlite: save default approval mode: %w", err)
		}
		return nil
	})
}

func (p *ModeStore) PlanModeActive(ctx context.Context, sessionID identity.SessionID) (bool, error) {
	return scopedValue(ctx, p.db.read, func(ctx context.Context) (bool, error) {
		if err := validateResource("read Session Plan mode", sessionID); err != nil {
			return false, err
		}
		var active bool
		if err := p.db.connection(ctx).QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM session_plan_modes WHERE session_id = ?)`, sessionID).Scan(&active); err != nil {
			return false, fmt.Errorf("sqlite: read session Plan mode: %w", err)
		}
		return active, nil
	})
}

func (p *ModeStore) StartPlanMode(ctx context.Context, sessionID identity.SessionID) (bool, error) {
	return scopedValue(ctx, p.db.write, func(ctx context.Context) (bool, error) {
		if err := validateResource("enter Session Plan mode", sessionID); err != nil {
			return false, err
		}
		result, err := p.db.connection(ctx).ExecContext(ctx, `INSERT INTO session_plan_modes(session_id) VALUES (?) ON CONFLICT(session_id) DO NOTHING`, sessionID)
		if err != nil {
			return false, fmt.Errorf("sqlite: enter session Plan mode: %w", err)
		}
		return rowChanged(result, "enter session Plan mode")
	})
}

func (p *ModeStore) EndPlanMode(ctx context.Context, sessionID identity.SessionID, revision uint64) (bool, error) {
	return scopedValue(ctx, p.db.write, func(ctx context.Context) (bool, error) {
		if err := validateResource("leave Session Plan mode", sessionID); err != nil {
			return false, err
		}
		result, err := p.db.connection(ctx).ExecContext(ctx, `DELETE FROM session_plan_modes WHERE session_id = ? AND EXISTS (
			SELECT 1 FROM session_plans p WHERE p.session_id = session_plan_modes.session_id AND p.revision = ?
		)`, sessionID, revision)
		if err != nil {
			return false, fmt.Errorf("sqlite: leave session Plan mode: %w", err)
		}
		return rowChanged(result, "leave session Plan mode")
	})
}

func (p *ModeStore) DeleteSession(ctx context.Context, sessionID identity.SessionID) error {
	return p.db.write(ctx, func(ctx context.Context) error {
		if err := validateResource("delete Session Plan mode", sessionID); err != nil {
			return err
		}
		_, err := p.db.connection(ctx).ExecContext(ctx, `DELETE FROM session_plan_modes WHERE session_id = ?`, sessionID)
		return err
	})
}

func rowChanged(result sql.Result, operation string) (bool, error) {
	changed, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("sqlite: %s: %w", operation, err)
	}
	return changed == 1, nil
}
