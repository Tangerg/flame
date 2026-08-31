package changefeed

import (
	"errors"
	"math"
)

type sequence struct {
	value uint64
}

// SequenceDisposition describes how one validated frame relates to the
// monotonic watermark owned by a subscription consumer.
type SequenceDisposition uint8

const (
	SequenceContiguous SequenceDisposition = iota + 1
	SequenceGap
	SequenceStale
)

// SequenceTracker owns the optional monotonic watermark for one subscription.
// Stale/duplicate frames never move it backwards; gaps advance it only to the
// newest observed frame. A new subscription receives a new tracker.
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
	if t.last.value != math.MaxUint64 && next == t.last.value+1 {
		t.last.value = next
		return SequenceContiguous, nil
	}
	t.last.value = next
	return SequenceGap, nil
}
