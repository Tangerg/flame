package persistence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/qimu/sqlite"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
)

// CommandAcceptanceStore writes immutable Run-owned receipts in their product
// transaction. Deleting original Run material retires its reference permanently;
// importing the same Run ID cannot bind that receipt to replacement material.
type commandAcceptanceStore struct{ db *sql.DB }

func newCommandAcceptanceStore(db *sql.DB) *commandAcceptanceStore {
	return &commandAcceptanceStore{db: db}
}

// Save records acceptance under the global command key. Re-observing a Steer
// receipt in later Scope heads is allowed only when its entire immutable value
// matches. Other reuse fails the surrounding product transaction.
func (s *commandAcceptanceStore) Save(ctx context.Context, acceptance runs.CommandAcceptance) error {
	if err := acceptance.Validate(); err != nil {
		return err
	}
	var steerInput []byte
	if acceptance.Operation == runs.CommandSteerInput {
		var err error
		steerInput, err = encodeUserContent(acceptance.SteerInput)
		if err != nil {
			return err
		}
		record, err := s.lookup(ctx, acceptance.Key)
		if err != nil {
			return err
		}
		if record.OtherOperation {
			return runs.ErrCommandKeyConflict
		}
		if record.Found {
			previous := record.Acceptance
			if !previous.Equal(acceptance) {
				return runs.ErrCommandKeyConflict
			}
			return nil
		}
		current, found, err := newRunStore(s.db).Run(ctx, acceptance.RunID)
		if err != nil {
			return err
		}
		if !found || current.SessionID() != acceptance.SessionID {
			return runs.ErrRunNotFound
		}
		if current.State() != run.Running {
			return runs.ErrRunFinished
		}
		if current.ActiveSegmentID() != acceptance.SegmentID {
			return runs.ErrStaleSegment
		}
	}
	_, err := sqlite.Connection(ctx, s.db).ExecContext(ctx,
		`INSERT INTO idempotency_records(
			key, kind, fingerprint, operation, session_id, run_id, segment_id, user_item_id, material_run_id, steer_input
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		acceptance.Key, idempotencyRunAcceptance, acceptance.Fingerprint, string(acceptance.Operation),
		acceptance.SessionID, acceptance.RunID, acceptance.SegmentID, acceptance.UserItemID,
		acceptance.RunID, steerInput,
	)
	if err != nil {
		return fmt.Errorf("sqlite: record command acceptance: %w", err)
	}
	return nil
}

// Lookup resolves a command record by its key. A run_acceptance projects the
// immutable acceptance and whether its original material is still referenced; a
// generic response variant reports that the key belongs to another operation; an
// absent key reports neither. It performs no mutation and starts no Run.
func (s *commandAcceptanceStore) Lookup(ctx context.Context, key string) (runs.CommandAcceptanceRecord, error) {
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

func (s *commandAcceptanceStore) SessionSteerInputs(ctx context.Context, sessionID string) ([]runs.CommandAcceptance, error) {
	rows, err := sqlite.Connection(ctx, s.db).QueryContext(ctx, `SELECT key FROM idempotency_records
		WHERE kind=? AND operation=? AND session_id=? AND material_run_id IS NOT NULL
		ORDER BY key`, idempotencyRunAcceptance, runs.CommandSteerInput, sessionID)
	if err != nil {
		return nil, err
	}
	var keys []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			_ = rows.Close()
			return nil, err
		}
		keys = append(keys, key)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return nil, err
	}
	var inputs []runs.CommandAcceptance
	for _, key := range keys {
		record, err := s.lookup(ctx, key)
		if err != nil {
			return nil, err
		}
		if !record.Found || record.CurrentRun == nil {
			return nil, errors.New("sqlite: accepted input changed inside its snapshot")
		}
		inputs = append(inputs, record.Acceptance)
	}
	return inputs, nil
}

func (s *commandAcceptanceStore) lookup(ctx context.Context, key string) (runs.CommandAcceptanceRecord, error) {
	var (
		kind        string
		digest      string
		operation   sql.NullString
		sessionID   sql.NullString
		runID       sql.NullString
		segmentID   sql.NullString
		userItemID  sql.NullString
		materialRun sql.NullString
		steerInput  []byte
	)
	err := sqlite.Connection(ctx, s.db).QueryRowContext(ctx,
		`SELECT kind, fingerprint, operation, session_id, run_id, segment_id, user_item_id, material_run_id, steer_input
		   FROM idempotency_records WHERE key = ?`, key,
	).Scan(&kind, &digest, &operation, &sessionID, &runID, &segmentID, &userItemID, &materialRun, &steerInput)
	if errors.Is(err, sql.ErrNoRows) {
		return runs.CommandAcceptanceRecord{}, nil
	}
	if err != nil {
		return runs.CommandAcceptanceRecord{}, fmt.Errorf("sqlite: look up command acceptance: %w", err)
	}
	if kind != idempotencyRunAcceptance {
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
	if len(steerInput) > 0 {
		acceptance.SteerInput, err = decodeUserContent(steerInput)
		if err != nil {
			return runs.CommandAcceptanceRecord{}, err
		}
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
	current, found, err := newRunStore(s.db).Run(ctx, materialRun.String)
	if err != nil {
		return runs.CommandAcceptanceRecord{}, err
	}
	if !found || current.SessionID() != acceptance.SessionID {
		return runs.CommandAcceptanceRecord{}, errors.New("sqlite: accepted Run material is missing or has another Session")
	}
	record.CurrentRun = &current
	if acceptance.Operation == runs.CommandSteerInput {
		item, found, err := newTranscriptStore(s.db).Item(ctx, acceptance.UserItemID)
		if err != nil {
			return runs.CommandAcceptanceRecord{}, err
		}
		if found {
			if item.RunID() != acceptance.RunID || item.SessionID() != acceptance.SessionID || item.Kind() != transcript.UserMessage || item.Status() != transcript.ItemCompleted {
				return runs.CommandAcceptanceRecord{}, errors.New("sqlite: steer application Item has another owner or kind")
			}
			record.AppliedItem = &item
		}
	}
	return record, nil
}
