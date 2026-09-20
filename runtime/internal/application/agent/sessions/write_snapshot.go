package sessions

import (
	"fmt"
	"slices"

	"github.com/Tangerg/scope/core/chat"

	"github.com/Tangerg/flame/runtime/internal/domain/run/conversation"
)

// ownWriteSnapshot normalizes and isolates one complete terminal projection
// before a Session write-set owns it.
func ownWriteSnapshot(snapshot Snapshot) (Snapshot, error) {
	normalized, err := snapshot.NormalizeForRestore()
	if err != nil {
		return Snapshot{}, fmt.Errorf("normalize snapshot: %w", err)
	}
	history, err := conversation.New(normalized.Messages)
	if err != nil {
		return Snapshot{}, fmt.Errorf("conversation: %w", err)
	}
	owned := Snapshot{
		Session: normalized.Session, Messages: history.Messages(),
		Runs: runsInParentFirstOrder(normalized.Runs), Items: slices.Clone(normalized.Items),
		ToolResults: slices.Clone(normalized.ToolResults), Plan: slices.Clone(normalized.Plan),
	}
	if err := owned.Validate(); err != nil {
		return Snapshot{}, err
	}
	return owned, nil
}

func cloneSnapshotMessages(messages []chat.Message) []chat.Message {
	owned := make([]chat.Message, len(messages))
	for index, message := range messages {
		owned[index] = message.Clone()
	}
	return owned
}
