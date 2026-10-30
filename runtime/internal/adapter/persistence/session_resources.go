package persistence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/qimu/sqlite"
	"github.com/Tangerg/flame/runtime/internal/application/agent/sessions"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
)

// ResourceRetirementStore retains cleanup authority after its Session history
// has been deleted or replaced. Only Session write-sets create these rows, in
// the transaction that decides that retirement.
type ResourceRetirementStore struct{ db *sql.DB }

func newResourceRetirementStore(db *sql.DB) *ResourceRetirementStore {
	return &ResourceRetirementStore{db: db}
}

func (s *ResourceRetirementStore) record(ctx context.Context, retirement sessions.ResourceRetirement) error {
	if err := retirement.Validate(); err != nil {
		return err
	}
	existing, found, err := s.Lookup(ctx, retirement.SessionID())
	if err != nil {
		return err
	}
	if found {
		if existing == retirement {
			return nil
		}
		return run.ErrSessionBusy
	}
	_, err = sqlite.Connection(ctx, s.db).ExecContext(ctx,
		`INSERT INTO committed_session_resource_retirements (retirement_id, session_id) VALUES (?, ?)`,
		retirement.ID(), retirement.SessionID())
	if err != nil {
		return fmt.Errorf("sqlite: record committed Session resource retirement: %w", err)
	}
	return nil
}

func (s *ResourceRetirementStore) Lookup(ctx context.Context, sessionID string) (sessions.ResourceRetirement, bool, error) {
	var id string
	err := sqlite.Connection(ctx, s.db).QueryRowContext(ctx,
		`SELECT retirement_id FROM committed_session_resource_retirements WHERE session_id = ?`, sessionID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return sessions.ResourceRetirement{}, false, nil
	}
	if err != nil {
		return sessions.ResourceRetirement{}, false, fmt.Errorf("sqlite: read Session resource retirement: %w", err)
	}
	retirement, err := sessions.RestoreResourceRetirement(id, sessionID)
	return retirement, err == nil, err
}

func (s *ResourceRetirementStore) Pending(ctx context.Context, sessionID string) (bool, error) {
	return sessionResourceRetirementPending(ctx, sqlite.Connection(ctx, s.db), sessionID)
}

func sessionResourceRetirementPending(ctx context.Context, connection sqlite.Executor, sessionID string) (bool, error) {
	var pending bool
	err := connection.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM committed_session_resource_retirements WHERE session_id = ?)`, sessionID).Scan(&pending)
	if err != nil {
		return false, fmt.Errorf("sqlite: inspect Session resource retirement: %w", err)
	}
	return pending, nil
}

func (s *ResourceRetirementStore) ListPending(ctx context.Context) ([]sessions.ResourceRetirement, error) {
	rows, err := sqlite.Connection(ctx, s.db).QueryContext(ctx,
		`SELECT retirement_id, session_id FROM committed_session_resource_retirements ORDER BY session_id`)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list Session resource retirements: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var retirements []sessions.ResourceRetirement
	for rows.Next() {
		var id, sessionID string
		if err := rows.Scan(&id, &sessionID); err != nil {
			return nil, fmt.Errorf("sqlite: scan Session resource retirement: %w", err)
		}
		retirement, err := sessions.RestoreResourceRetirement(id, sessionID)
		if err != nil {
			return nil, fmt.Errorf("sqlite: decode Session resource retirement: %w", err)
		}
		retirements = append(retirements, retirement)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: iterate Session resource retirements: %w", err)
	}
	return retirements, nil
}

func (s *ResourceRetirementStore) Complete(ctx context.Context, retirement sessions.ResourceRetirement) error {
	if err := retirement.Validate(); err != nil {
		return err
	}
	_, err := sqlite.Connection(ctx, s.db).ExecContext(ctx,
		`DELETE FROM committed_session_resource_retirements WHERE retirement_id = ? AND session_id = ?`,
		retirement.ID(), retirement.SessionID())
	if err != nil {
		return fmt.Errorf("sqlite: complete Session resource retirement: %w", err)
	}
	return nil
}
