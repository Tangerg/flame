package runs

import "github.com/Tangerg/flame/runtime/internal/domain/run/transcript"

// ItemWrite is one transcript Item a commit makes durable, in reduction order.
// A settled ToolCall carries its transition: its running state is durable only
// if a park or child admission wrote it, so persistence must be able to tell
// a missing predecessor from a raced one.
type ItemWrite struct {
	item       transcript.Item
	settlement *transcript.Replacement
}

// NewItemWrites appends each Item as a new transcript fact.
func NewItemWrites(items ...transcript.Item) []ItemWrite {
	writes := make([]ItemWrite, len(items))
	for index, item := range items {
		writes[index] = ItemWrite{item: item}
	}
	return writes
}

// SettlementWrite records a ToolCall settlement derived by the Item itself.
func SettlementWrite(settlement transcript.Replacement) ItemWrite {
	return ItemWrite{item: settlement.State(), settlement: &settlement}
}

// Item returns the Item state this write makes durable.
func (w ItemWrite) Item() transcript.Item { return w.item }

// Settlement returns the transition when this write settles an opened ToolCall.
func (w ItemWrite) Settlement() (transcript.Replacement, bool) {
	if w.settlement == nil {
		return transcript.Replacement{}, false
	}
	return *w.settlement, true
}
