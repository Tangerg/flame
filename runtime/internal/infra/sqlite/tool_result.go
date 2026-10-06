package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	"github.com/Tangerg/flame/runtime/internal/domain/run/toolresult"
)

// ToolResultStore is the single full-body source for oversized tool outputs.
// Stage creates an unbound row before the tool returns; the transcript Item
// whose offload names it binds it when the run's atomic event commit appends
// that Item.
type ToolResultStore struct {
	db *sql.DB
}

// NewToolResultStore wires a database with the current [Open]-installed schema
// to the offloaded-tool-result surface.
func NewToolResultStore(db *sql.DB) *ToolResultStore {
	return &ToolResultStore{db: db}
}

// Stage persists a body under its precomputed identity after the observer has
// verified that replacing it with a preview reduces model context.
func (t *ToolResultStore) Stage(ctx context.Context, stage toolresult.Stage) error {
	if err := stage.Validate(); err != nil {
		return fmt.Errorf("sqlite: stage tool result: %w", err)
	}
	_, err := conn(ctx, t.db).ExecContext(ctx,
		`INSERT INTO tool_result_blobs(id, session_id, body, created_at)
		 VALUES (?, ?, ?, strftime('%s','now'))`,
		stage.ID, stage.SessionID, stage.Body)
	if err != nil {
		return fmt.Errorf("sqlite: stage tool result %q: %w", stage.ID, err)
	}
	return nil
}

// Fetch returns the full offloaded body for (sessionID, id). found is false —
// with a nil error — when no such row exists (an unknown id is a recoverable
// miss the caller surfaces to the model, not a failure). Scoping the read by
// session id keeps one session from reading another's offloaded output.
func (t *ToolResultStore) Fetch(ctx context.Context, sessionID string, id toolresult.ID) (body string, found bool, err error) {
	if validateErr := resourceid.ValidateSession(sessionID); validateErr != nil {
		return "", false, fmt.Errorf("sqlite: fetch tool result: %w", validateErr)
	}
	if validateErr := id.Validate(); validateErr != nil {
		return "", false, fmt.Errorf("sqlite: fetch tool result: %w", validateErr)
	}
	err = conn(ctx, t.db).QueryRowContext(ctx,
		`SELECT body FROM tool_result_blobs WHERE id = ? AND session_id = ?`,
		id, sessionID).Scan(&body)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("sqlite: fetch tool result: %w", err)
	}
	return body, true, nil
}

// List returns every transcript-bound blob owned by sessionID in stable order.
func (t *ToolResultStore) List(ctx context.Context, sessionID string) ([]toolresult.Blob, error) {
	if err := resourceid.ValidateSession(sessionID); err != nil {
		return nil, fmt.Errorf("sqlite: list tool results: %w", err)
	}
	rows, err := conn(ctx, t.db).QueryContext(ctx,
		`SELECT id, session_id, body, created_at
		 FROM tool_result_blobs AS b
		 WHERE session_id = ? AND EXISTS (
		   SELECT 1 FROM history_items AS h WHERE h.offload_id = b.id AND h.session_id = b.session_id
		 )
		 ORDER BY created_at, id`, sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list tool results: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var blobs []toolresult.Blob
	for rows.Next() {
		var blob toolresult.Blob
		var rawID string
		var createdAt int64
		if scanErr := rows.Scan(&rawID, &blob.SessionID, &blob.Body, &createdAt); scanErr != nil {
			return nil, fmt.Errorf("sqlite: scan tool result: %w", scanErr)
		}
		blob.ID, err = toolresult.ParseID(rawID)
		if err != nil {
			return nil, fmt.Errorf("sqlite: decode tool-result ID %q: %w", rawID, err)
		}
		blob.CreatedAt = time.Unix(createdAt, 0).UTC()
		if err := blob.Validate(); err != nil {
			return nil, fmt.Errorf("sqlite: invalid stored tool result %q: %w", rawID, err)
		}
		blobs = append(blobs, blob)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: list tool results: %w", err)
	}
	return blobs, nil
}

// Restore inserts one artifact blob under its exact identity within its
// Session.
func (t *ToolResultStore) Restore(ctx context.Context, blob toolresult.Blob) error {
	if err := blob.Validate(); err != nil {
		return fmt.Errorf("sqlite: restore tool result: %w", err)
	}
	result, err := conn(ctx, t.db).ExecContext(ctx,
		`INSERT INTO tool_result_blobs(id, session_id, body, created_at)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(session_id, id) DO NOTHING`,
		blob.ID, blob.SessionID, blob.Body, blob.CreatedAt.Unix(),
	)
	if err != nil {
		return fmt.Errorf("sqlite: restore tool result %q: %w", blob.ID, err)
	}
	if inserted, err := result.RowsAffected(); err != nil {
		return fmt.Errorf("sqlite: inspect tool-result restore %q: %w", blob.ID, err)
	} else if inserted == 0 {
		return fmt.Errorf("%w: session %q already holds tool result %q", toolresult.ErrIdentityConflict, blob.SessionID, blob.ID)
	}
	return nil
}

// unboundToolResult selects bodies with neither an Item nor a checkpoint owner.
const unboundToolResult = `NOT EXISTS (SELECT 1 FROM history_items
	  WHERE session_id = tool_result_blobs.session_id AND offload_id = tool_result_blobs.id)
	AND NOT EXISTS (SELECT 1 FROM executor_checkpoint_tool_results
	  WHERE session_id = tool_result_blobs.session_id AND result_id = tool_result_blobs.id)`

// Discard removes only bodies with neither an Item nor a checkpoint owner.
// Failed publications must not invalidate a durable waiting continuation.
func (t *ToolResultStore) Discard(ctx context.Context, sessionID string, ref toolresult.Ref) error {
	if err := resourceid.ValidateSession(sessionID); err != nil {
		return fmt.Errorf("sqlite: discard tool result: %w", err)
	}
	if err := ref.Validate(); err != nil {
		return fmt.Errorf("sqlite: discard tool result: %w", err)
	}
	if _, err := conn(ctx, t.db).ExecContext(ctx,
		`DELETE FROM tool_result_blobs WHERE id = ? AND session_id = ? AND `+unboundToolResult,
		ref.ID, sessionID,
	); err != nil {
		return fmt.Errorf("sqlite: discard staged tool result %q: %w", ref.ID, err)
	}
	return nil
}

// PurgeUnbound removes abandoned staging bodies during startup, before new
// execution begins. Item and checkpoint references are both durable owners.
func (t *ToolResultStore) PurgeUnbound(ctx context.Context) (int64, error) {
	result, err := conn(ctx, t.db).ExecContext(ctx,
		`DELETE FROM tool_result_blobs WHERE `+unboundToolResult,
	)
	if err != nil {
		return 0, fmt.Errorf("sqlite: purge staged tool results: %w", err)
	}
	removed, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("sqlite: inspect staged tool-result purge: %w", err)
	}
	return removed, nil
}

// DropSession removes every offloaded body owned by sessionID — the blob half
// of the session-delete cascade. It joins an ambient lifecycle write-set
// transaction through conn(ctx).
func (t *ToolResultStore) DropSession(ctx context.Context, sessionID string) error {
	if err := resourceid.ValidateSession(sessionID); err != nil {
		return fmt.Errorf("sqlite: drop tool results: %w", err)
	}
	if _, err := conn(ctx, t.db).ExecContext(ctx,
		`DELETE FROM tool_result_blobs WHERE session_id = ?`, sessionID); err != nil {
		return fmt.Errorf("sqlite: drop session tool results: %w", err)
	}
	return nil
}
