package sqlite

import (
	"context"
	"database/sql"
	json "encoding/json/v2"
	"fmt"
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
	SessionID string
	Interrupt InterruptRecord
}

func (a *AcceptedContinuationStore) Put(ctx context.Context, record AcceptedContinuationRecord) error {
	payload, err := json.Marshal(record.Interrupt)
	if err != nil {
		return fmt.Errorf("sqlite: encode accepted continuation: %w", err)
	}
	_, err = Connection(ctx, a.db).ExecContext(ctx,
		`INSERT INTO accepted_continuations(root_run_id, session_id, interrupt)
		 VALUES (?, ?, ?)
		 ON CONFLICT(root_run_id) DO UPDATE SET
		   session_id = excluded.session_id,
		   interrupt = excluded.interrupt`,
		record.Interrupt.RootRunID, record.SessionID, string(payload))
	if err != nil {
		return fmt.Errorf("sqlite: put accepted continuation: %w", err)
	}
	return nil
}

func (a *AcceptedContinuationStore) Delete(ctx context.Context, rootRunID string) error {
	_, err := Connection(ctx, a.db).ExecContext(ctx,
		`DELETE FROM accepted_continuations WHERE root_run_id = ?`, rootRunID)
	if err != nil {
		return fmt.Errorf("sqlite: delete accepted continuation: %w", err)
	}
	return nil
}

func (a *AcceptedContinuationStore) List(ctx context.Context) ([]AcceptedContinuationRecord, error) {
	rows, err := Connection(ctx, a.db).QueryContext(ctx,
		`SELECT session_id, interrupt FROM accepted_continuations ORDER BY root_run_id`)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list accepted continuations: %w", err)
	}
	defer rows.Close()
	var records []AcceptedContinuationRecord
	for rows.Next() {
		var record AcceptedContinuationRecord
		var payload string
		if err := rows.Scan(&record.SessionID, &payload); err != nil {
			return nil, fmt.Errorf("sqlite: scan accepted continuation: %w", err)
		}
		if err := json.Unmarshal([]byte(payload), &record.Interrupt); err != nil {
			return nil, fmt.Errorf("sqlite: decode accepted continuation: %w", err)
		}
		records = append(records, record)
	}
	return records, rows.Err()
}
