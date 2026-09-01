package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
)

// PermissionModeStore persists the explicit permission state of sessions that
// entered Plan mode. Sessions without a row inherit the runtime default.
type PermissionModeStore struct{ db *sql.DB }

func NewPermissionModeStore(db *sql.DB) *PermissionModeStore {
	return &PermissionModeStore{db: db}
}

func (p *PermissionModeStore) LookupMode(ctx context.Context, sessionID string) (approval.SessionMode, bool, error) {
	if err := validateSessionResource("read Session permission mode", sessionID); err != nil {
		return approval.SessionMode{}, false, err
	}
	var state approval.SessionMode
	err := conn(ctx, p.db).QueryRowContext(ctx,
		`SELECT mode, restore_mode FROM session_permission_modes WHERE session_id = ?`, sessionID,
	).Scan(&state.Mode, &state.RestoreMode)
	if errors.Is(err, sql.ErrNoRows) {
		return approval.SessionMode{}, false, nil
	}
	if err != nil {
		return approval.SessionMode{}, false, fmt.Errorf("sqlite: read session permission mode: %w", err)
	}
	if err := state.Validate(); err != nil {
		return approval.SessionMode{}, false, fmt.Errorf("sqlite: validate session permission mode: %w", err)
	}
	return state, true, nil
}

func (p *PermissionModeStore) PutMode(ctx context.Context, sessionID string, state approval.SessionMode) error {
	if err := validateSessionResource("write Session permission mode", sessionID); err != nil {
		return err
	}
	if err := state.Validate(); err != nil {
		return fmt.Errorf("sqlite: validate session permission mode: %w", err)
	}
	if _, err := conn(ctx, p.db).ExecContext(ctx,
		`INSERT INTO session_permission_modes(session_id, mode, restore_mode) VALUES (?, ?, ?)
		 ON CONFLICT(session_id) DO UPDATE SET
		   mode = excluded.mode,
		   restore_mode = excluded.restore_mode`,
		sessionID, state.Mode, state.RestoreMode,
	); err != nil {
		return fmt.Errorf("sqlite: write session permission mode: %w", err)
	}
	return nil
}

// DeleteSession removes the explicit permission state owned by one Session.
// Session restore uses this before reseeding portable material: permission
// policy is local Runtime state and is deliberately not inherited from the
// Session that an imported archive replaces.
func (p *PermissionModeStore) DeleteSession(ctx context.Context, sessionID string) error {
	if err := validateSessionResource("delete Session permission mode", sessionID); err != nil {
		return err
	}
	if _, err := conn(ctx, p.db).ExecContext(ctx,
		`DELETE FROM session_permission_modes WHERE session_id = ?`, sessionID,
	); err != nil {
		return fmt.Errorf("sqlite: delete session permission mode: %w", err)
	}
	return nil
}
