package persistence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
)

// ToolResultStore reads the full bodies committed by TranscriptStore.
type ToolResultStore struct{ db *database }

func newToolResultStore(db *database) *ToolResultStore {
	return &ToolResultStore{db: db}
}

// Fetch returns the full offloaded body for (sessionID, id). found is false —
// with a nil error — when no such row exists (an unknown id is a recoverable
// miss the caller surfaces to the model, not a failure). Scoping the read by
// session id keeps one session from reading another's offloaded output.
func (t *ToolResultStore) Fetch(ctx context.Context, sessionID string, id transcript.ToolResultID) (body string, found bool, err error) {
	return scopedPair(ctx, t.db.read, func(ctx context.Context) (body string, found bool, err error) {
		if validateErr := identity.ValidateSession(sessionID); validateErr != nil {
			return "", false, fmt.Errorf("sqlite: fetch tool result: %w", validateErr)
		}
		if validateErr := id.Validate(); validateErr != nil {
			return "", false, fmt.Errorf("sqlite: fetch tool result: %w", validateErr)
		}
		err = t.db.connection(ctx).QueryRowContext(ctx, `SELECT body FROM tool_result_blobs WHERE id = ? AND session_id = ?`, id, sessionID).Scan(&body)
		if errors.Is(err, sql.ErrNoRows) {
			return "", false, nil
		}
		if err != nil {
			return "", false, fmt.Errorf("sqlite: fetch tool result: %w", err)
		}
		return body, true, nil
	})
}

// List returns every transcript-bound blob owned by sessionID in stable order.
func (t *ToolResultStore) List(ctx context.Context, sessionID string) ([]transcript.ToolResultBlob, error) {
	return scopedValue(ctx, t.db.read, func(ctx context.Context) ([]transcript.ToolResultBlob, error) {
		if err := identity.ValidateSession(sessionID); err != nil {
			return nil, fmt.Errorf("sqlite: list tool results: %w", err)
		}
		rows, err := t.db.connection(ctx).QueryContext(ctx, `SELECT id, session_id, body, created_at_ns
		 FROM tool_result_blobs
		 WHERE session_id = ?
		 ORDER BY created_at_ns, id`, sessionID)
		if err != nil {
			return nil, fmt.Errorf("sqlite: list tool results: %w", err)
		}
		defer func() {
			_ = rows.Close()
		}()
		var blobs []transcript.ToolResultBlob
		for rows.Next() {
			var blob transcript.ToolResultBlob
			var rawID string
			var createdAt int64
			if scanErr := rows.Scan(&rawID, &blob.SessionID, &blob.Body, &createdAt); scanErr != nil {
				return nil, fmt.Errorf("sqlite: scan tool result: %w", scanErr)
			}
			blob.ID, err = transcript.ParseToolResultID(rawID)
			if err != nil {
				return nil, fmt.Errorf("sqlite: decode tool-result ID %q: %w", rawID, err)
			}
			blob.CreatedAt = time.Unix(0, createdAt).UTC()
			if err := blob.Validate(); err != nil {
				return nil, fmt.Errorf("sqlite: invalid stored tool result %q: %w", rawID, err)
			}
			blobs = append(blobs, blob)
		}
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("sqlite: list tool results: %w", err)
		}
		return blobs, nil
	})
}
