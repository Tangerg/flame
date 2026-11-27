package persistence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
)

type commandAcceptanceStore struct{ db *database }

func newCommandAcceptanceStore(db *database) *commandAcceptanceStore {
	return &commandAcceptanceStore{db: db}
}

func (s *commandAcceptanceStore) Save(ctx context.Context, acceptance runs.CommandAcceptance) error {
	return s.db.write(ctx, func(ctx context.Context) error {
		if err := acceptance.Validate(); err != nil {
			return err
		}
		var steerInput []byte
		if acceptance.Operation == runs.CommandSteerInput {
			recorded, err := s.admitSteer(ctx, acceptance)
			if err != nil || recorded {
				return err
			}
			if steerInput, err = encodeUserContent(acceptance.SteerInput); err != nil {
				return err
			}
		}
		_, err := s.db.connection(ctx).ExecContext(ctx, `INSERT INTO idempotency_records(
			key, kind, fingerprint, operation, session_id, run_id, segment_id, user_item_id, material_run_id, steer_input
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, acceptance.Key, idempotencyRunAcceptance, acceptance.Fingerprint, string(acceptance.Operation), acceptance.SessionID, acceptance.RunID, acceptance.SegmentID, acceptance.UserItemID, acceptance.RunID, steerInput)
		if err != nil {
			return fmt.Errorf("sqlite: record command acceptance: %w", err)
		}
		return nil
	})
}

// Runs inside the recording write: the Steer use case checked the live Segment
// earlier, but only this transaction can see the Run the acceptance commits to.
func (s *commandAcceptanceStore) admitSteer(ctx context.Context, acceptance runs.CommandAcceptance) (recorded bool, err error) {
	record, err := s.Lookup(ctx, acceptance.Key)
	if err != nil {
		return false, err
	}
	if record.Foreign() {
		return false, runs.ErrCommandKeyConflict
	}
	if original, found := record.Accepted(); found {
		if !original.Equal(acceptance) {
			return false, runs.ErrCommandKeyConflict
		}
		return true, nil
	}
	current, found, err := newRunStore(s.db).Run(ctx, acceptance.RunID)
	if err != nil {
		return false, err
	}
	if !found || current.SessionID() != acceptance.SessionID {
		return false, runs.ErrRunNotFound
	}
	return false, runs.RequireRunningSegment(current, acceptance.SegmentID)
}

const commandAcceptanceColumns = `kind, key, fingerprint, operation, session_id, run_id, segment_id, user_item_id, material_run_id, steer_input`

func (s *commandAcceptanceStore) SessionSteerInputs(ctx context.Context, sessionID identity.SessionID) ([]runs.CommandAcceptance, error) {
	return scopedValue(ctx, s.db.read, func(ctx context.Context) ([]runs.CommandAcceptance, error) {
		rows, err := s.db.connection(ctx).QueryContext(ctx, `SELECT `+commandAcceptanceColumns+` FROM idempotency_records
		WHERE kind=? AND operation=? AND session_id=? AND material_run_id IS NOT NULL
		ORDER BY key`, idempotencyRunAcceptance, runs.CommandSteerInput, sessionID)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var inputs []runs.CommandAcceptance
		for rows.Next() {
			record, err := scanCommandAcceptance(rows)
			if err != nil {
				return nil, err
			}
			input, _ := record.Accepted()
			inputs = append(inputs, input)
		}
		return inputs, rows.Err()
	})
}

func (s *commandAcceptanceStore) Lookup(ctx context.Context, key string) (runs.CommandAcceptanceRecord, error) {
	return scopedValue(ctx, s.db.read, func(ctx context.Context) (runs.CommandAcceptanceRecord, error) {
		row := s.db.connection(ctx).QueryRowContext(ctx, `SELECT `+commandAcceptanceColumns+` FROM idempotency_records WHERE key=?`, key)
		record, err := scanCommandAcceptance(row)
		if errors.Is(err, sql.ErrNoRows) {
			return runs.CommandAcceptanceRecord{}, nil
		}
		if err != nil {
			return runs.CommandAcceptanceRecord{}, fmt.Errorf("sqlite: look up command acceptance: %w", err)
		}
		return record, nil
	})
}

func scanCommandAcceptance(row scanRow) (runs.CommandAcceptanceRecord, error) {
	var (
		kind        string
		operation   sql.NullString
		acceptance  runs.CommandAcceptance
		materialRun identity.RunID
		steerInput  []byte
	)
	if err := row.Scan(&kind, &acceptance.Key, &acceptance.Fingerprint, &operation, &acceptance.SessionID,
		&acceptance.RunID, &acceptance.SegmentID, &acceptance.UserItemID, &materialRun, &steerInput); err != nil {
		return runs.CommandAcceptanceRecord{}, err
	}
	if kind != idempotencyRunAcceptance {
		return runs.OccupiedCommandKey(), nil
	}
	acceptance.Operation = runs.CommandOperation(operation.String)
	if len(steerInput) > 0 {
		input, err := decodeUserContent(steerInput)
		if err != nil {
			return runs.CommandAcceptanceRecord{}, err
		}
		acceptance.SteerInput = input
	}
	if err := acceptance.Validate(); err != nil {
		return runs.CommandAcceptanceRecord{}, fmt.Errorf("sqlite: decode command acceptance: %w", err)
	}
	return runs.RecordedCommandAcceptance(acceptance, !materialRun.IsZero()), nil
}
