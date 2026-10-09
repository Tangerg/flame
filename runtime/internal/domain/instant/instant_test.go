package instant_test

import (
	"math"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/instant"
)

func TestValidatePreservesExactNanosecondBoundsAndAbsence(t *testing.T) {
	minimum, maximum := time.Unix(0, math.MinInt64).UTC(), time.Unix(0, math.MaxInt64).UTC()
	for _, value := range []time.Time{time.Time{}, minimum, maximum, time.Unix(0, 0), time.Unix(1, 123456789).In(time.FixedZone("offset", 3600))} {
		if err := instant.Validate(value); err != nil {
			t.Fatalf("valid instant %s: %v", value, err)
		}
		if !value.IsZero() && !time.Unix(0, value.UnixNano()).Equal(value) {
			t.Fatalf("accepted instant changed: %s", value)
		}
	}
	for _, value := range []time.Time{minimum.Add(-time.Nanosecond), maximum.Add(time.Nanosecond)} {
		if err := instant.Validate(value); err == nil {
			t.Fatalf("unrepresentable instant admitted: %s", value)
		}
	}
}
