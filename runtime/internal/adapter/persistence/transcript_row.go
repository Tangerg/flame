package persistence

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
)

const storedTranscriptColumns = `h.session_id, h.run_id, h.item_id, h.occurred_at, h.payload, COALESCE(b.id, ''), COALESCE(b.model_only, 0)`
const storedTranscriptFrom = `FROM history_items AS h LEFT JOIN tool_result_blobs AS b ON b.item_id = h.item_id`

type storedTranscriptRow struct {
	SessionID  identity.SessionID
	RunID      identity.RunID
	ItemID     identity.ItemID
	OccurredAt int64
	Payload    string
	OffloadID  string
	ModelOnly  bool
}

func encodeStoredTranscriptRow(item transcript.Item) (storedTranscriptRow, error) {
	payload, err := encodeTranscriptItem(item)
	if err != nil {
		return storedTranscriptRow{}, err
	}
	row := storedTranscriptRow{
		SessionID: item.SessionID(), RunID: item.RunID(), ItemID: item.ID(),
		OccurredAt: item.OccurredAt().UnixNano(), Payload: string(payload),
	}
	call, _ := item.ToolCall()
	if ref := call.Invocation.Offload; ref != nil {
		row.OffloadID, row.ModelOnly = ref.ID.String(), ref.ModelOnly
	}
	return row, nil
}

func (s *storedTranscriptRow) fields() []any {
	return []any{&s.SessionID, &s.RunID, &s.ItemID, &s.OccurredAt, &s.Payload, &s.OffloadID, &s.ModelOnly}
}

func (s *storedTranscriptRow) snapshot() (transcript.ItemSnapshot, error) {
	snapshot, err := decodeTranscriptItem([]byte(s.Payload))
	if err != nil {
		return transcript.ItemSnapshot{}, fmt.Errorf("sqlite: decode history item %q: %w", s.ItemID, err)
	}
	snapshot.Identity = transcript.ItemIdentity{SessionID: s.SessionID, RunID: s.RunID, ItemID: s.ItemID, OccurredAt: time.Unix(0, s.OccurredAt).UTC()}
	if s.OffloadID == "" {
		return snapshot, nil
	}
	id, err := transcript.ParseToolResultID(s.OffloadID)
	if err != nil {
		return transcript.ItemSnapshot{}, fmt.Errorf("sqlite: decode history item %q offload: %w", s.ItemID, err)
	}
	call, ok := snapshot.Body.(transcript.ToolCallBody)
	if !ok {
		return transcript.ItemSnapshot{}, fmt.Errorf("sqlite: history item %q has an offload identity but no tool invocation", s.ItemID)
	}
	call.Invocation.Offload = &transcript.ToolResultRef{ID: id, ModelOnly: s.ModelOnly}
	snapshot.Body = call
	return snapshot, nil
}

func (s *storedTranscriptRow) materialize(offloaded sql.NullString) (transcript.Item, error) {
	snapshot, err := s.snapshot()
	if err != nil {
		return transcript.Item{}, err
	}
	if call, ok := snapshot.Body.(transcript.ToolCallBody); ok && call.Invocation.Offload != nil {
		result, err := call.Invocation.Offload.HydrateResult(call.Invocation.Result, offloaded.String)
		if err != nil {
			return transcript.Item{}, fmt.Errorf("sqlite: hydrate history item %q: %w", s.ItemID, err)
		}
		call.Invocation.Result = &result
		snapshot.Body = call
	}
	item, err := transcript.RestoreItem(snapshot)
	if err != nil {
		return transcript.Item{}, fmt.Errorf("sqlite: decoded history item %q: %w", s.ItemID, err)
	}
	return item, nil
}

func scanTranscriptItem(row scanRow) (transcript.Item, error) {
	var stored storedTranscriptRow
	var offloaded sql.NullString
	if err := row.Scan(append(stored.fields(), &offloaded)...); err != nil {
		return transcript.Item{}, err
	}
	return stored.materialize(offloaded)
}
