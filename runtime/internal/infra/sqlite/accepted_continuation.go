package sqlite

import (
	"context"
	"database/sql"
	json "encoding/json/v2"
	"fmt"

	sqlitebase "github.com/Tangerg/flame/runtime/storage/sqlite"
)

// AcceptedContinuationStore persists the Run-owned binding that lets boot recovery
// revive an accepted but not-yet-delivered continuation. It stores the exact
// consumed interrupt hand-off so revival reconstructs the same tree; the answer
// resolutions stay in the transcript. Put joins the acceptance transaction through
// the context connection, so the binding and the answers it routes commit
// together. The DB must have been opened via [Open].
type AcceptedContinuationStore struct {
	db *sql.DB
}

func NewAcceptedContinuationStore(db *sql.DB) *AcceptedContinuationStore {
	return &AcceptedContinuationStore{db: db}
}

// AcceptedContinuationRecord is SQLite's technical representation: the Session of
// the accepted tree and the exact consumed interrupt hand-off it revives from.
type AcceptedContinuationRecord struct {
	SessionID   string
	Interrupt   InterruptRecord
	Segments    []AcceptedSegmentRecord
	InputItemID string
}

type AcceptedSegmentRecord struct {
	RunID     string `json:"run_id"`
	SegmentID string `json:"segment_id"`
}

func (a *AcceptedContinuationStore) Put(ctx context.Context, record AcceptedContinuationRecord) error {
	payload, err := json.Marshal(record.Interrupt)
	if err != nil {
		return fmt.Errorf("sqlite: encode accepted continuation: %w", err)
	}
	segments, err := json.Marshal(record.Segments)
	if err != nil {
		return err
	}
	_, err = sqlitebase.Connection(ctx, a.db).ExecContext(ctx,
		`INSERT INTO accepted_continuations(root_run_id, session_id, interrupt, segments, input_item_id)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(root_run_id) DO UPDATE SET
		   session_id = excluded.session_id,
		   interrupt = excluded.interrupt, segments = excluded.segments, input_item_id = excluded.input_item_id`,
		record.Interrupt.RootRunID, record.SessionID, string(payload), string(segments), record.InputItemID)
	if err != nil {
		return fmt.Errorf("sqlite: put accepted continuation: %w", err)
	}
	return nil
}

func (a *AcceptedContinuationStore) Delete(ctx context.Context, rootRunID string) error {
	_, err := sqlitebase.Connection(ctx, a.db).ExecContext(ctx,
		`DELETE FROM accepted_continuations WHERE root_run_id = ?`, rootRunID)
	if err != nil {
		return fmt.Errorf("sqlite: delete accepted continuation: %w", err)
	}
	return nil
}

func (a *AcceptedContinuationStore) List(ctx context.Context) ([]AcceptedContinuationRecord, error) {
	rows, err := sqlitebase.Connection(ctx, a.db).QueryContext(ctx,
		`SELECT root_run_id, session_id, interrupt, segments, input_item_id FROM accepted_continuations ORDER BY root_run_id`)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list accepted continuations: %w", err)
	}
	defer rows.Close()
	var records []AcceptedContinuationRecord
	for rows.Next() {
		var record AcceptedContinuationRecord
		var rootID, payload, segments string
		if err := rows.Scan(&rootID, &record.SessionID, &payload, &segments, &record.InputItemID); err != nil {
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
}
