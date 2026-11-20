package persistence

import (
	"context"
	json "encoding/json/v2"
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
)

// acceptedContinuationStore persists the Run-owned binding that lets boot recovery
// revive an accepted but not-yet-delivered continuation. It stores the exact
// consumed interrupt hand-off so revival reconstructs the same tree; the answer
// resolutions stay in the transcript. Put joins the acceptance transaction through
// the context connection, so the binding and the answers it routes commit
// together. The DB must have been opened via [Open].
type acceptedContinuationStore struct{ db *database }

func newAcceptedContinuationStore(db *database) *acceptedContinuationStore {
	return &acceptedContinuationStore{db: db}
}

// acceptedContinuationRecord is SQLite's technical representation: the Session of
// the accepted tree and the exact consumed interrupt hand-off it revives from.
type acceptedContinuationRecord struct {
	SessionID     identity.SessionID
	Interrupt     pendingRecord
	Segments      []acceptedSegmentRecord
	InputItemID   identity.ItemID
	CanceledRunID identity.RunID
}
type acceptedSegmentRecord struct {
	RunID     identity.RunID     `json:"run_id"`
	SegmentID identity.SegmentID `json:"segment_id"`
}

func (s *acceptedContinuationStore) saveContinuation(ctx context.Context, record acceptedContinuationRecord) error {
	return s.db.write(ctx, func(ctx context.Context) error {
		payload, err := json.Marshal(record.Interrupt)
		if err != nil {
			return fmt.Errorf("sqlite: encode accepted continuation: %w", err)
		}
		segments, err := json.Marshal(record.Segments)
		if err != nil {
			return err
		}
		_, err = s.db.connection(ctx).ExecContext(ctx, `INSERT INTO accepted_continuations
		  (root_run_id, session_id, interrupt, segments, input_item_id, canceled_run_id)
		  VALUES (?, ?, ?, ?, ?, NULLIF(?, ''))`,
			record.Interrupt.RootRunID, record.SessionID, string(payload), string(segments), record.InputItemID, record.CanceledRunID)
		if err != nil {
			return fmt.Errorf("sqlite: put accepted continuation: %w", err)
		}
		return nil
	})
}
func (s *acceptedContinuationStore) Delete(ctx context.Context, rootRunID identity.RunID) error {
	return s.db.write(ctx, func(ctx context.Context) error {
		_, err := s.db.connection(ctx).ExecContext(ctx, `DELETE FROM accepted_continuations WHERE root_run_id = ?`, rootRunID)
		if err != nil {
			return fmt.Errorf("sqlite: delete accepted continuation: %w", err)
		}
		return nil
	})
}

func (s *acceptedContinuationStore) DeleteSubtreeCancellation(ctx context.Context, rootRunID, targetRunID identity.RunID) error {
	return s.db.write(ctx, func(ctx context.Context) error {
		_, err := s.db.connection(ctx).ExecContext(ctx, `DELETE FROM accepted_continuations WHERE root_run_id = ? AND canceled_run_id = ?`, rootRunID, targetRunID)
		if err != nil {
			return fmt.Errorf("sqlite: retire accepted cancellation: %w", err)
		}
		return nil
	})
}

// continuationRecords reads the rows filter selects. A retained continuation and
// an accepted subtree cancellation share the table, told apart by whether the
// row names a canceled Run.
func (s *acceptedContinuationStore) continuationRecords(ctx context.Context, filter string, args ...any) ([]acceptedContinuationRecord, error) {
	return scopedValue(ctx, s.db.read, func(ctx context.Context) ([]acceptedContinuationRecord, error) {
		rows, err := s.db.connection(ctx).QueryContext(ctx, `SELECT root_run_id, session_id, interrupt, segments, input_item_id,
		  COALESCE(canceled_run_id, '') FROM accepted_continuations `+filter+` ORDER BY root_run_id`, args...)
		if err != nil {
			return nil, fmt.Errorf("sqlite: list accepted continuations: %w", err)
		}
		defer rows.Close()
		var records []acceptedContinuationRecord
		for rows.Next() {
			var record acceptedContinuationRecord
			var rootID identity.RunID
			var payload, segments string
			if err := rows.Scan(&rootID, &record.SessionID, &payload, &segments, &record.InputItemID, &record.CanceledRunID); err != nil {
				return nil, fmt.Errorf("sqlite: scan accepted continuation: %w", err)
			}
			if err := json.Unmarshal([]byte(payload), &record.Interrupt, json.RejectUnknownMembers(true)); err != nil {
				return nil, fmt.Errorf("sqlite: decode accepted continuation: %w", err)
			}
			if rootID != record.Interrupt.RootRunID {
				return nil, fmt.Errorf("sqlite: accepted continuation root identity mismatch")
			}
			if err := json.Unmarshal([]byte(segments), &record.Segments, json.RejectUnknownMembers(true)); err != nil {
				return nil, err
			}
			records = append(records, record)
		}
		return records, rows.Err()
	})
}
