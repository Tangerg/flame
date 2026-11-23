package persistence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
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
	switch record.Use {
	case runs.CommandKeyForeign:
		return false, runs.ErrCommandKeyConflict
	case runs.CommandKeyAccepted:
		if !record.Acceptance.Equal(acceptance) {
			return false, runs.ErrCommandKeyConflict
		}
		return true, nil
	case runs.CommandKeyUnused:
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

func (s *commandAcceptanceStore) SessionSteerInputs(ctx context.Context, sessionID identity.SessionID) ([]runs.CommandAcceptance, error) {
	return scopedValue(ctx, s.db.read, func(ctx context.Context) ([]runs.CommandAcceptance, error) {
		rows, err := s.db.connection(ctx).QueryContext(ctx, `SELECT key FROM idempotency_records
		WHERE kind=? AND operation=? AND session_id=? AND material_run_id IS NOT NULL
		ORDER BY key`, idempotencyRunAcceptance, runs.CommandSteerInput, sessionID)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var keys []string
		for rows.Next() {
			var key string
			if err := rows.Scan(&key); err != nil {
				return nil, err
			}
			keys = append(keys, key)
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		var inputs []runs.CommandAcceptance
		for _, key := range keys {
			record, err := s.Lookup(ctx, key)
			if err != nil {
				return nil, err
			}
			if record.Use != runs.CommandKeyAccepted || record.CurrentRun == nil {
				return nil, errors.New("sqlite: accepted input changed inside its snapshot")
			}
			inputs = append(inputs, record.Acceptance)
		}
		return inputs, nil
	})
}

func (s *commandAcceptanceStore) Lookup(ctx context.Context, key string) (runs.CommandAcceptanceRecord, error) {
	return scopedValue(ctx, s.db.read, func(ctx context.Context) (runs.CommandAcceptanceRecord, error) {
		var (
			kind        string
			digest      string
			operation   sql.NullString
			sessionID   identity.SessionID
			runID       identity.RunID
			segmentID   identity.SegmentID
			userItemID  identity.ItemID
			materialRun identity.RunID
			steerInput  []byte
		)
		err := s.db.connection(ctx).QueryRowContext(ctx, `SELECT kind, fingerprint, operation, session_id, run_id, segment_id, user_item_id, material_run_id, steer_input
		   FROM idempotency_records WHERE key = ?`, key).Scan(&kind, &digest, &operation, &sessionID, &runID, &segmentID, &userItemID, &materialRun, &steerInput)
		if errors.Is(err, sql.ErrNoRows) {
			return runs.CommandAcceptanceRecord{}, nil
		}
		if err != nil {
			return runs.CommandAcceptanceRecord{}, fmt.Errorf("sqlite: look up command acceptance: %w", err)
		}
		if kind != idempotencyRunAcceptance {
			return runs.CommandAcceptanceRecord{Use: runs.CommandKeyForeign}, nil
		}
		acceptance := runs.CommandAcceptance{Key: key, Fingerprint: digest, Operation: runs.CommandOperation(operation.String), SessionID: sessionID, RunID: runID, SegmentID: segmentID, UserItemID: userItemID}
		if len(steerInput) > 0 {
			acceptance.SteerInput, err = decodeUserContent(steerInput)
			if err != nil {
				return runs.CommandAcceptanceRecord{}, err
			}
		}
		if err := acceptance.Validate(); err != nil {
			return runs.CommandAcceptanceRecord{}, fmt.Errorf("sqlite: decode command acceptance: %w", err)
		}
		record := runs.CommandAcceptanceRecord{Use: runs.CommandKeyAccepted, Acceptance: acceptance}
		if materialRun.IsZero() {
			return record, nil
		}
		return s.withAcceptedMaterial(ctx, record, materialRun)
	})
}

func (s *commandAcceptanceStore) withAcceptedMaterial(ctx context.Context, record runs.CommandAcceptanceRecord, materialRun identity.RunID) (runs.CommandAcceptanceRecord, error) {
	acceptance := record.Acceptance
	current, found, err := newRunStore(s.db).Run(ctx, materialRun)
	if err != nil {
		return runs.CommandAcceptanceRecord{}, err
	}
	if !found || current.SessionID() != acceptance.SessionID {
		return runs.CommandAcceptanceRecord{}, errors.New("sqlite: accepted Run material is missing or has another Session")
	}
	record.CurrentRun = &current
	if acceptance.Operation != runs.CommandSteerInput {
		return record, nil
	}
	item, found, err := newTranscriptStore(s.db).Item(ctx, acceptance.UserItemID)
	if err != nil {
		return runs.CommandAcceptanceRecord{}, err
	}
	if !found {
		return record, nil
	}
	if item.RunID() != acceptance.RunID || item.SessionID() != acceptance.SessionID || item.Kind() != transcript.UserMessage || item.Status() != transcript.ItemCompleted {
		return runs.CommandAcceptanceRecord{}, errors.New("sqlite: steer application Item has another owner or kind")
	}
	record.AppliedItem = &item
	return record, nil
}
