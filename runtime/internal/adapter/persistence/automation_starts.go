package persistence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/infra/sqlite"
)

type AutomationStartStore struct{ db *sql.DB }

func NewAutomationStartStore(db *sql.DB) *AutomationStartStore { return &AutomationStartStore{db: db} }
func (s *AutomationStartStore) Get(ctx context.Context, id string) (runs.AutomationReceipt, bool, error) {
	var value runs.AutomationReceipt
	err := sqlite.Connection(ctx, s.db).QueryRowContext(ctx, `SELECT request_digest,session_id,run_id,segment_id FROM automation_starts WHERE run_id=?`, id).Scan(&value.RequestDigest, &value.SessionID, &value.RunID, &value.SegmentID)
	if errors.Is(err, sql.ErrNoRows) {
		return value, false, nil
	}
	if err != nil {
		return value, false, err
	}
	if err := value.Validate(); err != nil {
		return value, false, fmt.Errorf("sqlite: restore automated Run acceptance: %w", err)
	}
	return value, true, nil
}
func (s *AutomationStartStore) Save(ctx context.Context, value runs.AutomationReceipt) error {
	if err := value.Validate(); err != nil {
		return err
	}
	_, err := sqlite.Connection(ctx, s.db).ExecContext(ctx, `INSERT INTO automation_starts(run_id,request_digest,session_id,segment_id) VALUES(?,?,?,?)`, value.RunID, value.RequestDigest, value.SessionID, value.SegmentID)
	if err != nil {
		return fmt.Errorf("sqlite: record automated Run acceptance: %w", err)
	}
	return nil
}
