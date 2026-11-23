package persistence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
)

type TranscriptStore struct{ db *database }

func newTranscriptStore(db *database) *TranscriptStore {
	return &TranscriptStore{db: db}
}

func (t *TranscriptStore) AppendItem(ctx context.Context, item transcript.Item) error {
	return t.writeItem(ctx, item, appendPrecondition(), nil)
}

// A racing continuation's newer completion can never be overwritten by a
// cancellation built from an older parked projection.
func (t *TranscriptStore) ReplaceItem(ctx context.Context, change transcript.Replacement) error {
	return t.writeItem(ctx, change.State(), replacePrecondition(change.Expected()), change.ToolResult())
}

func (t *TranscriptStore) SettleItem(ctx context.Context, settlement transcript.Replacement) error {
	return t.writeItem(ctx, settlement.State(), settlePrecondition(settlement.Expected()), settlement.ToolResult())
}

type itemPrecondition struct {
	expected     *transcript.Item
	admitsAbsent bool
}

func appendPrecondition() itemPrecondition { return itemPrecondition{admitsAbsent: true} }

func replacePrecondition(expected transcript.Item) itemPrecondition {
	return itemPrecondition{expected: &expected}
}

func settlePrecondition(expected transcript.Item) itemPrecondition {
	return itemPrecondition{expected: &expected, admitsAbsent: true}
}

func (p itemPrecondition) admit(item, current transcript.Item, found bool) (bool, error) {
	switch {
	case !found && !p.admitsAbsent:
		return false, fmt.Errorf("sqlite: replace history item %q: not found", item.ID())
	case !found:
		return false, nil
	case p.expected == nil && current.Equal(item):
		return true, nil
	case p.expected == nil || !current.Equal(*p.expected):
		return false, fmt.Errorf("%w: item %q changed after its writer read it", transcript.ErrIdentityConflict, item.ID())
	default:
		return false, nil
	}
}

func (t *TranscriptStore) writeItem(ctx context.Context, item transcript.Item, precondition itemPrecondition, blob *transcript.ToolResultBlob) error {
	call, _ := item.ToolCall()
	ref := call.Invocation.Offload
	if (ref == nil) != (blob == nil) {
		return fmt.Errorf("sqlite: history item %q requires its offload reference and body together", item.ID())
	}
	if blob != nil {
		if err := blob.Validate(); err != nil {
			return fmt.Errorf("sqlite: history item %q offload: %w", item.ID(), err)
		}
		if blob.ID != ref.ID || blob.SessionID != item.SessionID() {
			return fmt.Errorf("sqlite: history item %q offload body has a different identity", item.ID())
		}
	}
	payload, err := encodeTranscriptItem(item)
	if err != nil {
		return fmt.Errorf("sqlite: encode history item: %w", err)
	}
	return t.db.write(ctx, func(ctx context.Context) error {
		current, found, err := t.storedItem(ctx, item.ID())
		if err != nil {
			return err
		}
		written, err := precondition.admit(item, current, found)
		if err != nil {
			return err
		}
		if written {
			return t.writeToolResult(ctx, item.ID(), ref, blob)
		}
		_, err = t.db.connection(ctx).ExecContext(ctx, `INSERT INTO history_items(session_id, run_id, item_id, occurred_at, payload)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(item_id) DO UPDATE SET payload = excluded.payload`, item.SessionID(), item.RunID(), item.ID(), item.OccurredAt().UnixNano(), string(payload))
		if err != nil {
			return fmt.Errorf("sqlite: write history item %q: %w", item.ID(), err)
		}
		return t.writeToolResult(ctx, item.ID(), ref, blob)
	})
}

func (t *TranscriptStore) storedItem(ctx context.Context, itemID identity.ItemID) (transcript.Item, bool, error) {
	return scopedPair(ctx, t.db.read, func(ctx context.Context) (transcript.Item, bool, error) {
		var stored storedTranscriptRow
		row := t.db.connection(ctx).QueryRowContext(ctx, `SELECT `+storedTranscriptColumns+` `+storedTranscriptFrom+` WHERE h.item_id = ?`, itemID)
		err := row.Scan(stored.fields()...)
		if errors.Is(err, sql.ErrNoRows) {
			return transcript.Item{}, false, nil
		}
		if err != nil {
			return transcript.Item{}, false, fmt.Errorf("sqlite: inspect history item: %w", err)
		}
		snapshot, err := stored.snapshot()
		if err != nil {
			return transcript.Item{}, false, err
		}
		item, err := transcript.RestoreItem(snapshot)
		if err != nil {
			return transcript.Item{}, false, fmt.Errorf("sqlite: decoded history item %q: %w", itemID, err)
		}
		return item, true, nil
	})
}

func (t *TranscriptStore) writeToolResult(ctx context.Context, itemID identity.ItemID, ref *transcript.ToolResultRef, blob *transcript.ToolResultBlob) error {
	if blob == nil {
		return nil
	}
	q := t.db.connection(ctx)
	createdAt := blob.CreatedAt.UnixNano()
	result, err := q.ExecContext(ctx, `INSERT INTO tool_result_blobs(item_id, session_id, id, model_only, body, created_at_ns)
	 VALUES (?, ?, ?, ?, ?, ?) ON CONFLICT DO NOTHING`, itemID, blob.SessionID, ref.ID, ref.ModelOnly, blob.Body, createdAt)
	if err != nil {
		return fmt.Errorf("sqlite: write tool result %q: %w", ref.ID, err)
	}
	written, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite: inspect tool result write: %w", err)
	}
	if written == 1 {
		return nil
	}
	var same bool
	if err := q.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM tool_result_blobs
	 WHERE item_id = ? AND session_id = ? AND id = ? AND model_only = ? AND body = ? AND created_at_ns = ?)`,
		itemID, blob.SessionID, ref.ID, ref.ModelOnly, blob.Body, createdAt).Scan(&same); err != nil {
		return fmt.Errorf("sqlite: inspect tool result conflict: %w", err)
	}
	if !same {
		return fmt.Errorf("%w: offload %q conflicts with the body or owner in session %q", transcript.ErrToolResultIdentityConflict, ref.ID, blob.SessionID)
	}
	return nil
}

func (t *TranscriptStore) Item(ctx context.Context, itemID identity.ItemID) (transcript.Item, bool, error) {
	return scopedPair(ctx, t.db.read, func(ctx context.Context) (transcript.Item, bool, error) {
		if err := itemID.Validate(); err != nil {
			return transcript.Item{}, false, fmt.Errorf("sqlite: history item: %w", err)
		}
		row := t.db.connection(ctx).QueryRowContext(ctx, `SELECT `+storedTranscriptColumns+`, b.body `+storedTranscriptFrom+`
		  WHERE h.item_id = ?`, itemID)
		item, err := scanTranscriptItem(row)
		if errors.Is(err, sql.ErrNoRows) {
			return transcript.Item{}, false, nil
		}
		if err != nil {
			return transcript.Item{}, false, err
		}
		return item, true, nil
	})
}

func (t *TranscriptStore) DeleteRun(ctx context.Context, sessionID identity.SessionID, runID identity.RunID) error {
	if err := validateResource("delete history Run", sessionID); err != nil {
		return err
	}
	if err := validateResource("delete history Run", runID); err != nil {
		return err
	}
	return t.db.write(ctx, func(ctx context.Context) error {
		if _, err := t.db.connection(ctx).ExecContext(ctx, `DELETE FROM history_items WHERE session_id = ? AND run_id = ?`, sessionID, runID); err != nil {
			return fmt.Errorf("sqlite: delete run items: %w", err)
		}
		return nil
	})
}

func (t *TranscriptStore) DeleteSession(ctx context.Context, sessionID identity.SessionID) error {
	if err := validateResource("delete history Session", sessionID); err != nil {
		return err
	}
	return t.db.write(ctx, func(ctx context.Context) error {
		q := t.db.connection(ctx)
		if _, err := q.ExecContext(ctx, `DELETE FROM history_items WHERE session_id = ?`, sessionID); err != nil {
			return fmt.Errorf("sqlite: delete session items: %w", err)
		}
		return nil
	})
}

func (t *TranscriptStore) List(ctx context.Context, sessionID identity.SessionID) ([]transcript.Item, error) {
	sequenced, err := t.PageSessionItems(ctx, sessionID, transcript.OldestFirst, 0, 0)
	if err != nil {
		return nil, err
	}
	out := make([]transcript.Item, 0, len(sequenced))
	for _, entry := range sequenced {
		out = append(out, entry.Item)
	}
	return out, nil
}

func (t *TranscriptStore) ListStored(ctx context.Context, sessionID identity.SessionID) ([]transcript.Item, error) {
	return scopedValue(ctx, t.db.read, func(ctx context.Context) ([]transcript.Item, error) {
		rows, err := t.db.connection(ctx).QueryContext(ctx, `SELECT `+storedTranscriptColumns+` `+storedTranscriptFrom+`
		  WHERE h.session_id = ?
		  ORDER BY h.seq`, sessionID)
		if err != nil {
			return nil, fmt.Errorf("sqlite: list stored history items: %w", err)
		}
		defer rows.Close()
		var out []transcript.Item
		for rows.Next() {
			var stored storedTranscriptRow
			if err := rows.Scan(stored.fields()...); err != nil {
				return nil, fmt.Errorf("sqlite: scan stored history item: %w", err)
			}
			snapshot, err := stored.snapshot()
			if err != nil {
				return nil, err
			}
			item, err := transcript.RestoreItem(snapshot)
			if err != nil {
				return nil, fmt.Errorf("sqlite: decoded history item %q: %w", stored.itemID, err)
			}
			out = append(out, item)
		}
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("sqlite: list stored history items: %w", err)
		}
		return out, nil
	})
}

func (t *TranscriptStore) PageSessionItems(ctx context.Context, sessionID identity.SessionID, order transcript.SequenceOrder, fromSequence int64, limit int) ([]transcript.SequencedItem, error) {
	if err := validateResource("page Session history", sessionID); err != nil {
		return nil, err
	}
	return t.pageItems(ctx, `h.session_id = ?`, sessionID, order, fromSequence, limit)
}

func (t *TranscriptStore) PageRunItems(ctx context.Context, runID identity.RunID, order transcript.SequenceOrder, fromSequence int64, limit int) ([]transcript.SequencedItem, error) {
	if err := validateResource("page Run history", runID); err != nil {
		return nil, err
	}
	return t.pageItems(ctx, `h.run_id = ?`, runID, order, fromSequence, limit)
}

func (t *TranscriptStore) PageRunTreeItems(ctx context.Context, runID identity.RunID, order transcript.SequenceOrder, fromSequence int64, limit int) ([]transcript.SequencedItem, error) {
	if err := validateResource("page Run-tree history", runID); err != nil {
		return nil, err
	}
	return t.pageItems(ctx, `h.run_id IN (
		WITH RECURSIVE subtree(run_id) AS (
			SELECT run_id
			  FROM runs
			 WHERE run_id = ?
			UNION
			SELECT child.run_id
			  FROM runs AS child
			  JOIN subtree AS parent ON child.parent_run_id = parent.run_id
		)
		SELECT run_id FROM subtree
	)`, runID, order, fromSequence, limit)
}

func (t *TranscriptStore) pageItems(ctx context.Context, scope string, subject any, order transcript.SequenceOrder, fromSequence int64, limit int) ([]transcript.SequencedItem, error) {
	return scopedValue(ctx, t.db.read, func(ctx context.Context) ([]transcript.SequencedItem, error) {
		if err := order.Validate(); err != nil {
			return nil, fmt.Errorf("sqlite: page history items: %w", err)
		}
		if fromSequence < 0 {
			return nil, errors.New("sqlite: history page sequence must not be negative")
		}
		if limit < 0 {
			return nil, errors.New("sqlite: history page limit must not be negative")
		}
		query := `SELECT h.seq, ` + storedTranscriptColumns + `, b.body ` + storedTranscriptFrom + ` WHERE ` + scope
		args := []any{subject}
		bound, direction := `h.seq > ?`, ``
		if order == transcript.NewestFirst {
			bound, direction = `h.seq < ?`, ` DESC`
		}
		if fromSequence > 0 {
			query += ` AND ` + bound
			args = append(args, fromSequence)
		}
		query += ` ORDER BY h.seq` + direction
		if limit > 0 {
			query += ` LIMIT ?`
			args = append(args, limit)
		}
		rows, err := t.db.connection(ctx).QueryContext(ctx, query, args...)
		if err != nil {
			return nil, fmt.Errorf("sqlite: list history items: %w", err)
		}
		defer rows.Close()
		var out []transcript.SequencedItem
		for rows.Next() {
			var stored storedTranscriptRow
			var offloadedBody sql.NullString
			var sequence int64
			fields := append([]any{&sequence}, stored.fields()...)
			fields = append(fields, &offloadedBody)
			if err := rows.Scan(fields...); err != nil {
				return nil, fmt.Errorf("sqlite: scan history item: %w", err)
			}
			item, err := stored.materialize(offloadedBody)
			if err != nil {
				return nil, err
			}
			out = append(out, transcript.SequencedItem{Sequence: sequence, Item: item})
		}
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("sqlite: list history items: %w", err)
		}
		return out, nil
	})
}
