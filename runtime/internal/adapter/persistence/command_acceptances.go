package persistence

import (
	"context"
	"database/sql"
	"errors"
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

// Lookup resolves a command record by its key. A run_opening projects the
// immutable acceptance and whether its original material is still referenced; a
// generic response variant reports that the key belongs to another operation; an
// absent key reports neither. It performs no mutation and starts no Run.
func (s *CommandAcceptanceStore) Lookup(ctx context.Context, key string) (runs.CommandAcceptanceRecord, error) {
	var record runs.CommandAcceptanceRecord
	err := sqlite.RunInTx(ctx, s.db, func(ctx context.Context) error {
		var err error
		record, err = s.lookup(ctx, key)
		return err
	})
	if err != nil {
		return runs.CommandAcceptanceRecord{}, err
	}
	return record, nil
}

func (s *CommandAcceptanceStore) lookup(ctx context.Context, key string) (runs.CommandAcceptanceRecord, error) {
	var (
		kind        string
		digest      string
		operation   sql.NullString
		sessionID   sql.NullString
		runID       sql.NullString
		segmentID   sql.NullString
		userItemID  sql.NullString
		materialRun sql.NullString
	)
	err := sqlite.Connection(ctx, s.db).QueryRowContext(ctx,
		`SELECT kind, fingerprint, operation, session_id, run_id, segment_id, user_item_id, material_run_id
		   FROM idempotency_records WHERE key = ?`, key,
	).Scan(&kind, &digest, &operation, &sessionID, &runID, &segmentID, &userItemID, &materialRun)
	if errors.Is(err, sql.ErrNoRows) {
		return runs.CommandAcceptanceRecord{}, nil
	}
	if err != nil {
		return runs.CommandAcceptanceRecord{}, fmt.Errorf("sqlite: look up command acceptance: %w", err)
	}
	if kind != "run_opening" {
		return runs.CommandAcceptanceRecord{OtherOperation: true}, nil
	}
	acceptance := runs.CommandAcceptance{
		Key:         key,
		Fingerprint: digest,
		Operation:   runs.CommandOperation(operation.String),
		SessionID:   sessionID.String,
		RunID:       runID.String,
		SegmentID:   segmentID.String,
		UserItemID:  userItemID.String,
	}
	if err := acceptance.Validate(); err != nil {
		return runs.CommandAcceptanceRecord{}, fmt.Errorf("sqlite: decode command acceptance: %w", err)
	}
	record := runs.CommandAcceptanceRecord{Acceptance: acceptance, Found: true}
	if !materialRun.Valid {
		return record, nil
	}
	if materialRun.String != acceptance.RunID {
		return runs.CommandAcceptanceRecord{}, errors.New("sqlite: command material differs from its accepted Run")
	}
	current, found, err := sqlite.NewRunStore(s.db).Run(ctx, materialRun.String)
	if err != nil {
		return runs.CommandAcceptanceRecord{}, err
	}
	if !found || current.SessionID() != acceptance.SessionID {
		return runs.CommandAcceptanceRecord{}, errors.New("sqlite: accepted Run material is missing or has another Session")
	}
	record.CurrentRun = &current
	return record, nil
}
