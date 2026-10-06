package sessions

import (
	"fmt"
	"slices"

	"github.com/Tangerg/scope/core/chat"

	"github.com/Tangerg/flame/runtime/internal/domain/run/conversation"
)

// ownWriteSnapshot isolates one complete terminal projection before a Session
// write-set owns it.
func ownWriteSnapshot(snapshot Snapshot) (Snapshot, error) {
	history, err := conversation.New(snapshot.Messages)
	if err != nil {
		return Snapshot{}, fmt.Errorf("conversation: %w", err)
	}
	owned := Snapshot{
		Session: snapshot.Session, Messages: history.Messages(),
		Runs: runsInParentFirstOrder(snapshot.Runs), Items: slices.Clone(snapshot.Items),
		ToolResults: slices.Clone(snapshot.ToolResults), Plan: slices.Clone(snapshot.Plan),
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
