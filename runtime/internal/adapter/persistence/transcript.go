package persistence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/qimu/sqlite"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
)

type TranscriptStore struct{ db *sql.DB }

func newTranscriptStore(db *sql.DB) *TranscriptStore { return &TranscriptStore{db: db} }

// AppendItem records a new Item. Re-appending the identical Item is a no-op;
// any other Item under the same identity is a conflict, because only a named
// transition may change a durable Item.
func (t *TranscriptStore) AppendItem(ctx context.Context, item transcript.Item) error {
	return t.writeItem(ctx, item, nil, true)
}

// ReplaceItem atomically replaces expected with its decided state only while
// the durable Item still equals the immutable value the application planned
// against. This is the transcript-side compare-and-swap used by tree
// transformations: a racing continuation can never have its newer completion
// overwritten by a cancellation built from an older parked projection.
func (t *TranscriptStore) ReplaceItem(ctx context.Context, change transcript.Replacement) error {
	expected := change.Expected()
	return t.writeItem(ctx, change.State(), &expected, false)
}

// SettleItem applies a ToolCall settlement. Its running state is durable only
// when a park or child admission wrote it, so an absent row means that state
// never left the executor; a present row must still equal it.
func (t *TranscriptStore) SettleItem(ctx context.Context, settlement transcript.Replacement) error {
	expected := settlement.Expected()
	return t.writeItem(ctx, settlement.State(), &expected, true)
}

func (t *TranscriptStore) writeItem(ctx context.Context, item transcript.Item, expected *transcript.Item, absentInserts bool) error {
	offloadID, err := transcriptOffloadID(item)
	if err != nil {
		return err
	}
	payload, err := encodeTranscriptItem(item)
	if err != nil {
		return fmt.Errorf("sqlite: encode history item: %w", err)
	}
	return sqlite.RunInTx(ctx, t.db, func(ctx context.Context) error {
		current, found, err := t.storedItem(ctx, item.ID())
		if err != nil {
			return err
		}
		switch {
		case !found && !absentInserts:
			return fmt.Errorf("sqlite: replace history item %q: not found", item.ID())
		case found && expected == nil && current.Equal(item):
			return nil
		case found && (expected == nil || !current.Equal(*expected)):
			return fmt.Errorf("%w: item %q changed after its writer read it", transcript.ErrIdentityConflict, item.ID())
		}
		return t.writeItemRecord(ctx, item, payload, offloadID)
	})
}

func transcriptOffloadID(item transcript.Item) (transcript.ToolResultID, error) {
	call, present := item.ToolCall()
	invocation := call.Invocation
	if !present || invocation.Offload == nil {
		return "", nil
	}
	if err := invocation.Offload.Validate(); err != nil {
		return "", fmt.Errorf("sqlite: history item offload: %w", err)
	}
	if invocation.Result == nil {
		return "", errors.New("sqlite: offloaded history item result is absent")
	}
	if _, ok := invocation.Result.String(); !ok {
		return "", errors.New("sqlite: offloaded history item result must be a preview string")
	}
	return invocation.Offload.ID, nil
}

// storedItem reads one Item as written, with an offloaded result still its
// preview, which is the form every writer holds.
func (t *TranscriptStore) storedItem(ctx context.Context, itemID string) (transcript.Item, bool, error) {
	var sessionID, runID, payload, offloadID string
	var occurredAt int64
	err := sqlite.Connection(ctx, t.db).QueryRowContext(ctx,
		`SELECT session_id, run_id, occurred_at, payload, offload_id FROM history_items WHERE item_id = ?`,
		itemID,
	).Scan(&sessionID, &runID, &occurredAt, &payload, &offloadID)
	if errors.Is(err, sql.ErrNoRows) {
		return transcript.Item{}, false, nil
	}
	if err != nil {
		return transcript.Item{}, false, fmt.Errorf("sqlite: inspect history item: %w", err)
	}
	snapshot, err := storedTranscriptItem(sessionID, runID, itemID, occurredAt, payload, offloadID)
	if err != nil {
		return transcript.Item{}, false, err
	}
	item, err := transcript.RestoreItem(snapshot)
	if err != nil {
		return transcript.Item{}, false, fmt.Errorf("sqlite: decoded history item %q: %w", itemID, err)
	}
	return item, true, nil
}

func (t *TranscriptStore) writeItemRecord(
	ctx context.Context,
	item transcript.Item,
	payload []byte,
	offloadID transcript.ToolResultID,
) error {
	q := sqlite.Connection(ctx, t.db)
	if offloadID != "" {
		var staged bool
		if err := q.QueryRowContext(ctx,
			`SELECT EXISTS (SELECT 1 FROM tool_result_blobs WHERE id = ? AND session_id = ?)`,
			offloadID, item.SessionID(),
		).Scan(&staged); err != nil {
			return fmt.Errorf("sqlite: inspect history item offload: %w", err)
		}
		if !staged {
			return fmt.Errorf("sqlite: history item %q offloads to %q, which session %q does not hold", item.ID(), offloadID, item.SessionID())
		}
	}
	result, err := q.ExecContext(ctx,
		`INSERT INTO history_items(session_id, run_id, item_id, occurred_at, payload, offload_id)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(item_id) DO UPDATE SET
		   payload = excluded.payload,
		   offload_id = excluded.offload_id
		 WHERE history_items.session_id = excluded.session_id
		   AND history_items.run_id = excluded.run_id
		   AND history_items.occurred_at = excluded.occurred_at
		   AND (history_items.offload_id = '' OR history_items.offload_id = excluded.offload_id)`,
		item.SessionID(), item.RunID(), item.ID(), item.OccurredAt().UnixNano(), string(payload), offloadID,
	)
	if err != nil {
		return t.explainItemAppendError(ctx, item.SessionID(), item.ID(), offloadID, err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite: inspect history item write: %w", err)
	}
	if changed != 1 {
		return fmt.Errorf("%w: item %q already belongs to another session, run, occurrence, or offload identity", transcript.ErrIdentityConflict, item.ID())
	}
	return nil
}

func (t *TranscriptStore) explainItemAppendError(
	ctx context.Context,
	sessionID, itemID string,
	offloadID transcript.ToolResultID,
	appendErr error,
) error {
	if offloadID == "" {
		return fmt.Errorf("sqlite: append history item: %w", appendErr)
	}
	var ownerItemID string
	ownerErr := sqlite.Connection(ctx, t.db).QueryRowContext(ctx,
		`SELECT item_id FROM history_items WHERE session_id = ? AND offload_id = ?`, sessionID, offloadID,
	).Scan(&ownerItemID)
	if ownerErr == nil && ownerItemID != itemID {
		return fmt.Errorf("%w: offload %q already belongs to item %q", transcript.ErrIdentityConflict, offloadID, ownerItemID)
	}
	if ownerErr != nil && !errors.Is(ownerErr, sql.ErrNoRows) {
		return fmt.Errorf("sqlite: inspect history item offload conflict: %w", errors.Join(appendErr, ownerErr))
	}
	return fmt.Errorf("sqlite: append history item: %w", appendErr)
}

// Item resolves one durable transcript Item by its globally unique identity.
// The returned value is the same fully hydrated projection as List/Page reads.
func (t *TranscriptStore) Item(ctx context.Context, itemID string) (transcript.Item, bool, error) {
	if err := identity.ValidateItem(itemID); err != nil {
		return transcript.Item{}, false, fmt.Errorf("sqlite: history item: %w", err)
	}
	row := sqlite.Connection(ctx, t.db).QueryRowContext(ctx,
		`SELECT session_id, run_id, item_id, occurred_at, payload, offload_id,
		        (SELECT body FROM tool_result_blobs WHERE id = history_items.offload_id
		          AND session_id = history_items.session_id)
		   FROM history_items
		  WHERE item_id = ?`,
		itemID,
	)
	item, err := scanTranscriptItem(row)
	if errors.Is(err, sql.ErrNoRows) {
		return transcript.Item{}, false, nil
	}
	if err != nil {
		return transcript.Item{}, false, err
	}
	return item, true, nil
}

func scanTranscriptItem(row scanRow) (transcript.Item, error) {
	var (
		sessionID    string
		runID        string
		itemID       string
		payload      string
		rawOffloadID string
		offloaded    sql.NullString
		occurredAt   int64
	)
	if err := row.Scan(
		&sessionID,
		&runID,
		&itemID,
		&occurredAt,
		&payload,
		&rawOffloadID,
		&offloaded,
	); err != nil {
		return transcript.Item{}, err
	}
	return materializeTranscriptItem(sessionID, runID, itemID, occurredAt, payload, rawOffloadID, offloaded)
}

func materializeTranscriptItem(
	sessionID, runID, itemID string,
	occurredAt int64,
	payload, rawOffloadID string,
	offloaded sql.NullString,
) (transcript.Item, error) {
	snapshot, err := storedTranscriptItem(sessionID, runID, itemID, occurredAt, payload, rawOffloadID)
	if err != nil {
		return transcript.Item{}, err
	}
	if call, ok := snapshot.Body.(transcript.ToolCallBody); ok && call.Invocation.Offload != nil {
		if !offloaded.Valid {
			return transcript.Item{}, fmt.Errorf("sqlite: history item %q references missing tool result %q", itemID, call.Invocation.Offload.ID)
		}
		body, err := tool.NewResult(offloaded.String)
		if err != nil {
			return transcript.Item{}, fmt.Errorf("sqlite: decode history item %q offloaded text: %w", itemID, err)
		}
		call.Invocation.Result = &body
		snapshot.Body = call
	}
	item, err := transcript.RestoreItem(snapshot)
	if err != nil {
		return transcript.Item{}, fmt.Errorf("sqlite: decoded history item %q: %w", itemID, err)
	}
	return item, nil
}

// storedTranscriptItem decodes one row as written: an offloaded Tool result
// keeps the preview that replaced its body.
func storedTranscriptItem(
	sessionID, runID, itemID string,
	occurredAt int64,
	payload, rawOffloadID string,
) (transcript.ItemSnapshot, error) {
	snapshot, err := decodeTranscriptItem([]byte(payload))
	if err != nil {
		return transcript.ItemSnapshot{}, fmt.Errorf("sqlite: decode history item %q: %w", itemID, err)
	}
	snapshot.Identity = transcript.ItemIdentity{
		SessionID: sessionID, RunID: runID, ItemID: itemID,
		OccurredAt: time.Unix(0, occurredAt).UTC(),
	}
	if rawOffloadID == "" {
		return snapshot, nil
	}
	id, err := transcript.ParseToolResultID(rawOffloadID)
	if err != nil {
		return transcript.ItemSnapshot{}, fmt.Errorf("sqlite: decode history item %q offload: %w", itemID, err)
	}
	call, ok := snapshot.Body.(transcript.ToolCallBody)
	if !ok {
		return transcript.ItemSnapshot{}, fmt.Errorf("sqlite: history item %q has an offload identity but no tool invocation", itemID)
	}
	if call.Invocation.Result == nil {
		return transcript.ItemSnapshot{}, fmt.Errorf("sqlite: history item %q has an offload identity but no preview", itemID)
	}
	if _, ok := call.Invocation.Result.String(); !ok {
		return transcript.ItemSnapshot{}, fmt.Errorf("sqlite: history item %q has an offload identity but no preview string", itemID)
	}
	call.Invocation.Offload = &transcript.ToolResultRef{ID: id}
	snapshot.Body = call
	return snapshot, nil
}

// DeleteRun removes one run's items from a session's history. The Run's own row
// belongs to the run store; this store owns the item log.
func (t *TranscriptStore) DeleteRun(ctx context.Context, sessionID, runID string) error {
	if err := validateSessionResource("delete history Run", sessionID); err != nil {
		return err
	}
	if err := validateRunResource("delete history Run", runID); err != nil {
		return err
	}
	return sqlite.RunInTx(ctx, t.db, func(ctx context.Context) error {
		q := sqlite.Connection(ctx, t.db)
		if _, err := q.ExecContext(ctx,
			`DELETE FROM tool_result_blobs
			 WHERE session_id = ? AND id IN (
			   SELECT offload_id FROM history_items WHERE session_id = ? AND run_id = ? AND offload_id != ''
			 )`, sessionID, sessionID, runID,
		); err != nil {
			return fmt.Errorf("sqlite: delete run tool results: %w", err)
		}
		if _, err := q.ExecContext(ctx,
			`DELETE FROM history_items WHERE session_id = ? AND run_id = ?`, sessionID, runID,
		); err != nil {
			return fmt.Errorf("sqlite: delete run items: %w", err)
		}
		return nil
	})
}

func (t *TranscriptStore) DeleteSession(ctx context.Context, sessionID string) error {
	if err := validateSessionResource("delete history Session", sessionID); err != nil {
		return err
	}
	return sqlite.RunInTx(ctx, t.db, func(ctx context.Context) error {
		q := sqlite.Connection(ctx, t.db)
		if _, err := q.ExecContext(ctx, `DELETE FROM history_items WHERE session_id = ?`, sessionID); err != nil {
			return fmt.Errorf("sqlite: delete session items: %w", err)
		}
		return nil
	})
}

// List returns a session's whole item history in durable append order.
func (t *TranscriptStore) List(ctx context.Context, sessionID string) ([]transcript.Item, error) {
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
func (t *TranscriptStore) ListStored(ctx context.Context, sessionID string) ([]transcript.Item, error) {
	rows, err := sqlite.Connection(ctx, t.db).QueryContext(ctx,
		`SELECT session_id, run_id, item_id, occurred_at, payload, offload_id
		   FROM history_items
		  WHERE session_id = ?
		  ORDER BY seq`,
		sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list stored history items: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []transcript.Item
	for rows.Next() {
		var session, runID, itemID, payload, rawOffloadID string
		var occurredAt int64
		if err := rows.Scan(&session, &runID, &itemID, &occurredAt, &payload, &rawOffloadID); err != nil {
			return nil, fmt.Errorf("sqlite: scan stored history item: %w", err)
		}
		snapshot, err := storedTranscriptItem(session, runID, itemID, occurredAt, payload, rawOffloadID)
		if err != nil {
			return nil, err
		}
		item, err := transcript.RestoreItem(snapshot)
		if err != nil {
			return nil, fmt.Errorf("sqlite: decoded history item %q: %w", itemID, err)
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: list stored history items: %w", err)
	}
	return out, nil
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
func (t *TranscriptStore) PageSessionItems(ctx context.Context, sessionID string, order transcript.SequenceOrder, fromSequence int64, limit int) ([]transcript.SequencedItem, error) {
	if err := validateSessionResource("page Session history", sessionID); err != nil {
		return nil, err
	}
	return t.pageItems(ctx, `h.session_id = ?`, sessionID, order, fromSequence, limit)
}

// PageRunItems is the same page over one run's own items. The run id needs no
// session beside it: it identifies exactly one run, and a run belongs to one
// session.
func (t *TranscriptStore) PageRunItems(ctx context.Context, runID string, order transcript.SequenceOrder, fromSequence int64, limit int) ([]transcript.SequencedItem, error) {
	if err := validateRunResource("page Run history", runID); err != nil {
		return nil, err
	}
	return t.pageItems(ctx, `h.run_id = ?`, runID, order, fromSequence, limit)
}

// PageRunTreeItems returns one Run's items plus every descendant's, using the
// durable parent edge as the subtree authority. The transcript never infers
// lineage from event order or spawning-item contents.
func (t *TranscriptStore) PageRunTreeItems(ctx context.Context, runID string, order transcript.SequenceOrder, fromSequence int64, limit int) ([]transcript.SequencedItem, error) {
	if err := validateRunResource("page Run-tree history", runID); err != nil {
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

func (t *TranscriptStore) pageItems(ctx context.Context, scope, subject string, order transcript.SequenceOrder, fromSequence int64, limit int) ([]transcript.SequencedItem, error) {
	if err := order.Validate(); err != nil {
		return nil, fmt.Errorf("sqlite: page history items: %w", err)
	}
	if fromSequence < 0 {
		return nil, errors.New("sqlite: history page sequence must not be negative")
	}
	if limit < 0 {
		return nil, errors.New("sqlite: history page limit must not be negative")
	}
	query := `SELECT h.seq, h.session_id, h.run_id, h.item_id, h.occurred_at, h.payload, h.offload_id, b.body
		 FROM history_items AS h
		 LEFT JOIN tool_result_blobs AS b
		   ON b.id = h.offload_id AND b.session_id = h.session_id
		 WHERE ` + scope
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
	rows, err := sqlite.Connection(ctx, t.db).QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list history items: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []transcript.SequencedItem
	for rows.Next() {
		var session, runID, itemID, payload, rawOffloadID string
		var offloadedBody sql.NullString
		var sequence, occurredAt int64
		if err := rows.Scan(&sequence, &session, &runID, &itemID, &occurredAt, &payload, &rawOffloadID, &offloadedBody); err != nil {
			return nil, fmt.Errorf("sqlite: scan history item: %w", err)
		}
		item, err := materializeTranscriptItem(session, runID, itemID, occurredAt, payload, rawOffloadID, offloadedBody)
		if err != nil {
			return nil, err
		}
		out = append(out, transcript.SequencedItem{Sequence: sequence, Item: item})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: list history items: %w", err)
	}
	return out, nil
}
