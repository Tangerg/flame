package sessions

import (
	"slices"

	"github.com/Tangerg/scope/core/chat"
)

// ownHistory isolates one complete terminal projection before a Session
// write-set owns it.
func ownHistory(snapshot HistorySnapshot) (HistorySnapshot, error) {
	if err := snapshot.validate(); err != nil {
		return HistorySnapshot{}, err
	}
	owned := snapshot.clone()
	sortRunsParentFirst(owned.Runs)
	return owned, nil
}

func (s HistorySnapshot) clone() HistorySnapshot {
	return HistorySnapshot{
		Session: s.Session, Messages: cloneSnapshotMessages(s.Messages),
		Runs: slices.Clone(s.Runs), Items: slices.Clone(s.Items),
		ToolResults: slices.Clone(s.ToolResults),
	}
}

func cloneSnapshotMessages(messages []chat.Message) []chat.Message {
	owned := make([]chat.Message, len(messages))
	for index, message := range messages {
		owned[index] = message.Clone()
	}
	return owned
}
