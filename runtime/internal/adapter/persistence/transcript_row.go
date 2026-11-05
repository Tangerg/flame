package persistence

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
)

const storedTranscriptColumns = `h.session_id, h.run_id, h.item_id, h.occurred_at, h.payload, h.offload_id, h.offload_model_only`

type storedTranscriptRow struct {
	sessionID  string
	runID      string
	itemID     string
	occurredAt int64
	payload    string
	offloadID  string
	modelOnly  bool
}

func (s *storedTranscriptRow) fields() []any {
	return []any{&s.sessionID, &s.runID, &s.itemID, &s.occurredAt, &s.payload, &s.offloadID, &s.modelOnly}
}

func (s storedTranscriptRow) snapshot() (transcript.ItemSnapshot, error) {
	snapshot, err := decodeTranscriptItem([]byte(s.payload))
	if err != nil {
		return transcript.ItemSnapshot{}, fmt.Errorf("sqlite: decode history item %q: %w", s.itemID, err)
	}
	snapshot.Identity = transcript.ItemIdentity{SessionID: s.sessionID, RunID: s.runID, ItemID: s.itemID, OccurredAt: time.Unix(0, s.occurredAt).UTC()}
	if s.offloadID == "" {
		if s.modelOnly {
			return transcript.ItemSnapshot{}, fmt.Errorf("sqlite: history item %q has an offload mode but no identity", s.itemID)
		}
		return snapshot, nil
	}
	id, err := transcript.ParseToolResultID(s.offloadID)
	if err != nil {
		return transcript.ItemSnapshot{}, fmt.Errorf("sqlite: decode history item %q offload: %w", s.itemID, err)
	}
	call, ok := snapshot.Body.(transcript.ToolCallBody)
	if !ok {
		return transcript.ItemSnapshot{}, fmt.Errorf("sqlite: history item %q has an offload identity but no tool invocation", s.itemID)
	}
	ref := transcript.ToolResultRef{ID: id, ModelOnly: s.modelOnly}
	if err := ref.ValidateResult(call.Invocation.Result); err != nil {
		return transcript.ItemSnapshot{}, fmt.Errorf("sqlite: history item %q: %w", s.itemID, err)
	}
	call.Invocation.Offload = &ref
	snapshot.Body = call
	return snapshot, nil
}

func (s storedTranscriptRow) materialize(offloaded sql.NullString) (transcript.Item, error) {
	snapshot, err := s.snapshot()
	if err != nil {
		return transcript.Item{}, err
	}
	if call, ok := snapshot.Body.(transcript.ToolCallBody); ok && call.Invocation.Offload != nil {
		if !offloaded.Valid {
			return transcript.Item{}, fmt.Errorf("sqlite: history item %q references missing tool result %q", s.itemID, call.Invocation.Offload.ID)
		}
		result, err := call.Invocation.Offload.HydrateResult(call.Invocation.Result, offloaded.String)
		if err != nil {
			return transcript.Item{}, fmt.Errorf("sqlite: hydrate history item %q: %w", s.itemID, err)
		}
		call.Invocation.Result = &result
		snapshot.Body = call
	}
	item, err := transcript.RestoreItem(snapshot)
	if err != nil {
		return transcript.Item{}, fmt.Errorf("sqlite: decoded history item %q: %w", s.itemID, err)
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
