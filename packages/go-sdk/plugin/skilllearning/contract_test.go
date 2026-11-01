package skilllearning

import (
	"testing"
	"time"
)

func TestArchiveAdviceRequiresAPastCutoff(t *testing.T) {
	now := time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC)
	for _, cutoff := range []*time.Time{nil, new(now.Add(-time.Nanosecond))} {
		if err := (ArchiveAdvice{InactiveSince: cutoff}).Validate(now); err != nil {
			t.Fatal(err)
		}
	}
	for _, cutoff := range []time.Time{{}, now, now.Add(time.Nanosecond)} {
		if err := (ArchiveAdvice{InactiveSince: &cutoff}).Validate(now); err == nil {
			t.Fatalf("invalid cutoff: %s", cutoff)
		}
	}
}
