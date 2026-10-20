package changefeed

import (
	"math"
	"testing"
)

func TestSequenceTrackerKeepsAMonotonicWatermark(t *testing.T) {
	t.Parallel()
	tracker := NewSequenceTracker()
	for _, test := range []struct {
		sequence uint64
		want     SequenceDisposition
	}{
		{sequence: 1, want: SequenceContiguous},
		{sequence: 2, want: SequenceContiguous},
		{sequence: 4, want: SequenceGap},
		{sequence: 3, want: SequenceStale},
		{sequence: 4, want: SequenceStale},
		{sequence: 5, want: SequenceContiguous},
	} {
		got, err := tracker.Observe(test.sequence)
		if err != nil || got != test.want {
			t.Fatalf("Observe(%d) = (%v, %v), want %v", test.sequence, got, err, test.want)
		}
	}
}

func TestSequenceTrackerRejectsZeroAndDoesNotWrapAtMaximum(t *testing.T) {
	t.Parallel()
	tracker := NewSequenceTracker()
	if _, err := tracker.Observe(0); err == nil {
		t.Fatal("zero sequence was accepted")
	}
	if got, err := tracker.Observe(math.MaxUint64); err != nil || got != SequenceGap {
		t.Fatalf("maximum first sequence = (%v, %v)", got, err)
	}
	if got, err := tracker.Observe(math.MaxUint64 - 1); err != nil || got != SequenceStale {
		t.Fatalf("sequence after maximum = (%v, %v)", got, err)
	}
}
