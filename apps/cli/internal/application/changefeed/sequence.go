package changefeed

import "errors"

type sequence struct {
	value uint64
}

type SequenceDisposition uint8

const (
	SequenceContiguous SequenceDisposition = iota + 1
	SequenceGap
	SequenceStale
)

type SequenceTracker struct {
	last *sequence
}

func NewSequenceTracker() *SequenceTracker { return &SequenceTracker{} }

func (t *SequenceTracker) Observe(next uint64) (SequenceDisposition, error) {
	if next == 0 {
		return 0, errors.New("change event sequence is zero")
	}
	if t.last == nil {
		t.last = &sequence{value: next}
		if next == 1 {
			return SequenceContiguous, nil
		}
		return SequenceGap, nil
	}
	if next <= t.last.value {
		return SequenceStale, nil
	}
	// next is already greater than the watermark, so the successor cannot wrap.
	if next == t.last.value+1 {
		t.last.value = next
		return SequenceContiguous, nil
	}
	t.last.value = next
	return SequenceGap, nil
}
