package runs

import "github.com/Tangerg/flame/runtime/internal/domain/run/transcript"

type ItemWrite struct {
	item       transcript.Item
	settlement *transcript.Replacement
}

func NewItemWrites(items ...transcript.Item) []ItemWrite {
	writes := make([]ItemWrite, len(items))
	for index, item := range items {
		writes[index] = ItemWrite{item: item}
	}
	return writes
}

func SettlementWrite(settlement transcript.Replacement) ItemWrite {
	return ItemWrite{item: settlement.State(), settlement: &settlement}
}

func (w ItemWrite) Item() transcript.Item { return w.item }

func (w ItemWrite) Settlement() (transcript.Replacement, bool) {
	if w.settlement == nil {
		return transcript.Replacement{}, false
	}
	return *w.settlement, true
}
