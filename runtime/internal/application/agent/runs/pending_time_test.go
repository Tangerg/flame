package runs

import (
	"math"
	"testing"
	"time"
)

func TestPendingRequiresExactDurableTime(t *testing.T) {
	for _, at := range []time.Time{time.Unix(0, math.MinInt64).UTC(), time.Unix(0, 0).UTC(), time.Unix(0, math.MaxInt64).UTC()} {
		pending := validTreePending()
		pending.CreatedAt = at
		if err := pending.Validate(); err != nil {
			t.Fatalf("Pending at %s: %v", at, err)
		}
	}
	for _, at := range []time.Time{time.Unix(0, math.MinInt64).Add(-time.Nanosecond).UTC(), time.Unix(0, math.MaxInt64).Add(time.Nanosecond).UTC()} {
		pending := validTreePending()
		pending.CreatedAt = at
		if err := pending.Validate(); err == nil {
			t.Errorf("Pending at %s succeeded", at)
		}
	}
}
