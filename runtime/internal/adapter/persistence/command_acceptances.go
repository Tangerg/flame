package persistence

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/infra/sqlite"
)

// CommandAcceptanceStore persists the Run-owned run_opening variant of the
// shared command-record table. The acceptance is written inside the Run opening
// transaction, so its global key constraint and material reference commit
// atomically with the Run it names; a lost response can never split "this
// command was accepted" from "this Run was opened".
type CommandAcceptanceStore struct{ db *sql.DB }

func NewCommandAcceptanceStore(db *sql.DB) *CommandAcceptanceStore {
	return &CommandAcceptanceStore{db: db}
}

// Save records one immutable command acceptance. The material reference starts
// as the accepted Run; the material's deletion retires it to NULL through the
// foreign key. A primary-key conflict means the global key is already held, so
// the enclosing opening transaction fails rather than opening a second Run under
// the same identity.
func (s *CommandAcceptanceStore) Save(ctx context.Context, acceptance runs.CommandAcceptance) error {
	if err := acceptance.Validate(); err != nil {
		return err
	}
	_, err := sqlite.Connection(ctx, s.db).ExecContext(ctx,
		`INSERT INTO idempotency_records(
			key, kind, fingerprint, operation, session_id, run_id, segment_id, user_item_id, material_run_id
		) VALUES (?, 'run_opening', ?, ?, ?, ?, ?, ?, ?)`,
		acceptance.Key, acceptance.Fingerprint, string(acceptance.Operation),
		acceptance.SessionID, acceptance.RunID, acceptance.SegmentID, acceptance.UserItemID,
		acceptance.RunID,
	)
	if err != nil {
		return fmt.Errorf("sqlite: record command acceptance: %w", err)
	}
	return nil
}
