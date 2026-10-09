package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/feedback"
)

// FeedbackStore persists the append-only quality ledger. The DB must have been
// opened via Open so the feedback_entries table exists.
type FeedbackStore struct {
	db *sql.DB
}

// NewFeedbackStore wires db to the feedback receiver.
func NewFeedbackStore(db *sql.DB) *FeedbackStore {
	return &FeedbackStore{db: db}
}

// Append stores a validated immutable feedback entry. It joins any ambient
// lifecycle write set through conn, even though feedback is normally an
// independent user action.
func (f *FeedbackStore) Append(ctx context.Context, entry feedback.Entry) error {
	if err := entry.Validate(); err != nil {
		return err
	}
	if _, err := conn(ctx, f.db).ExecContext(ctx,
		`INSERT INTO feedback_entries(session_id, run_id, item_id, rating, text, created_at_ns)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		entry.SessionID, entry.RunID, entry.ItemID, entry.Rating, entry.Text, entry.CreatedAt.UnixNano()); err != nil {
		return fmt.Errorf("sqlite: append feedback: %w", err)
	}
	return nil
}

// ListSession includes observations whose optional references name the Session
// or its retained Runs and Items. References remain user claims, not foreign keys.
func (f *FeedbackStore) ListSession(ctx context.Context, sessionID string) ([]feedback.Entry, error) {
	rows, err := conn(ctx, f.db).QueryContext(ctx,
		`SELECT session_id, run_id, item_id, rating, text, created_at_ns FROM feedback_entries WHERE `+
			sessionFeedbackPredicate+` ORDER BY created_at_ns, id`, sessionID, sessionID, sessionID)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list Session feedback: %w", err)
	}
	defer func() { _ = rows.Close() }()
	entries := make([]feedback.Entry, 0)
	for rows.Next() {
		var entry feedback.Entry
		var createdAt int64
		if err := rows.Scan(&entry.SessionID, &entry.RunID, &entry.ItemID, &entry.Rating, &entry.Text, &createdAt); err != nil {
			return nil, fmt.Errorf("sqlite: scan Session feedback: %w", err)
		}
		entry.CreatedAt = time.Unix(0, createdAt).UTC()
		if err := entry.Validate(); err != nil {
			return nil, fmt.Errorf("sqlite: restore Session feedback: %w", err)
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: iterate Session feedback: %w", err)
	}
	return entries, nil
}
