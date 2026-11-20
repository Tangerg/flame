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

// AppendItem records a new Item. Re-appending the identical Item is a no-op;
// any other Item under the same identity is a conflict, because only a named
// transition may change a durable Item.
func (t *TranscriptStore) AppendItem(ctx context.Context, item transcript.Item) error {
	return t.writeItem(ctx, item, appendPrecondition(), nil)
}

// ReplaceItem atomically replaces expected with its decided state only while
// the durable Item still equals the immutable value the application planned
// against. This is the transcript-side compare-and-swap used by tree
// transformations: a racing continuation can never have its newer completion
// overwritten by a cancellation built from an older parked projection.
func (t *TranscriptStore) ReplaceItem(ctx context.Context, change transcript.Replacement) error {
	return t.writeItem(ctx, change.State(), replacePrecondition(change.Expected()), change.ToolResult())
}

// SettleItem applies a ToolCall settlement. Its running state is durable only
// when a park or child admission wrote it, so an absent row means that state
// never left the executor; a present row must still equal it.
func (t *TranscriptStore) SettleItem(ctx context.Context, settlement transcript.Replacement) error {
	return t.writeItem(ctx, settlement.State(), settlePrecondition(settlement.Expected()), settlement.ToolResult())
}

// itemPrecondition is the durable row a writer planned against: none for a new
// Item, otherwise the exact Item it read. A settlement also admits an absent
// row because its running state is durable only when a park or child admission
// wrote it.
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

// admit reports whether the durable row already holds item, or why the planned
// write no longer applies.
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

// storedItem reads one Item as written, with an offloaded result still its
// preview, which is the form every writer holds.
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

// Item resolves one durable transcript Item by its globally unique identity.
// The returned value is the same fully hydrated projection as List/Page reads.
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

// DeleteRun removes one run's items from a session's history. The Run's own row
// belongs to the run store; this store owns the item log.
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

// List returns a session's whole item history in durable append order.
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

// ListStored returns a session's Items as written, in durable append order:
// an offloaded Tool result keeps its preview instead of the joined body. A
// portable Session snapshot captures this form beside the bodies it offloads.
func (t *TranscriptStore) ListStored(ctx context.Context, sessionID identity.SessionID) ([]transcript.Item, error) {
	return scopedValue(ctx, t.db.read, func(ctx context.Context) ([]transcript.Item, error) {
		rows, err := t.db.connection(ctx).QueryContext(ctx, `SELECT `+storedTranscriptColumns+` `+storedTranscriptFrom+`
		  WHERE h.session_id = ?
		  ORDER BY h.seq`, sessionID)
		if err != nil {
			return nil, fmt.Errorf("sqlite: list stored history items: %w", err)
		}
		defer func() {
			_ = rows.Close()
		}()
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

// PageSessionItems returns one page of a session's history along the durable
// sequence, in the direction order names. fromSequence is the position a previous
// page ended at; zero is no anchor, which is exact because the sequence is
// 1-based — so the same zero means "from the beginning" reading forwards and "from
// the end" reading backwards. A zero limit reads to the end.
//
// The bound is applied by the query, not by the caller: seeking past an anchor and
// stopping at a limit is what keeps a long session's history out of memory when
// only a page of it was asked for.
func (t *TranscriptStore) PageSessionItems(ctx context.Context, sessionID identity.SessionID, order transcript.SequenceOrder, fromSequence int64, limit int) ([]transcript.SequencedItem, error) {
	if err := validateResource("page Session history", sessionID); err != nil {
		return nil, err
	}
	return t.pageItems(ctx, `h.session_id = ?`, sessionID, order, fromSequence, limit)
}

// PageRunItems is the same page over one run's own items. The run id needs no
// session beside it: it identifies exactly one run, and a run belongs to one
// session.
func (t *TranscriptStore) PageRunItems(ctx context.Context, runID identity.RunID, order transcript.SequenceOrder, fromSequence int64, limit int) ([]transcript.SequencedItem, error) {
	if err := validateResource("page Run history", runID); err != nil {
		return nil, err
	}
	return t.pageItems(ctx, `h.run_id = ?`, runID, order, fromSequence, limit)
}

// PageRunTreeItems returns one Run's items plus every descendant's, using the
// durable parent edge as the subtree authority. The transcript never infers
// lineage from event order or spawning-item contents.
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
		defer func() {
			_ = rows.Close()
		}()
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
