package plan

import (
	"errors"
	"math"
	"testing"
	"time"
)

func TestPlanTimesRemainExactlyRepresentable(t *testing.T) {
	for _, at := range []time.Time{time.Unix(0, math.MinInt64).UTC(), time.Unix(0, 0).UTC(), time.Unix(0, math.MaxInt64).UTC()} {
		state, err := (Current{}).Replace(nil, at)
		if err != nil || !state.UpdatedAt().Equal(at) {
			t.Fatalf("Replace at %s = %v, %v", at, state.UpdatedAt(), err)
		}
	}
	for _, at := range []time.Time{time.Unix(0, math.MinInt64).Add(-time.Nanosecond), time.Unix(0, math.MaxInt64).Add(time.Nanosecond)} {
		if _, err := (Current{}).Replace(nil, at); !errors.Is(err, ErrInvalid) {
			t.Errorf("initial Replace at %s = %v, want ErrInvalid", at, err)
		}
		if _, err := Restore(Snapshot{Revision: 1, UpdatedAt: at}); !errors.Is(err, ErrInvalid) {
			t.Errorf("Restore at %s = %v, want ErrInvalid", at, err)
		}
	}
	state, err := (Current{}).Replace(nil, time.Unix(0, math.MaxInt64).UTC())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.Replace(nil, state.UpdatedAt().Add(time.Nanosecond)); !errors.Is(err, ErrInvalid) {
		t.Errorf("replacement past exact range = %v, want ErrInvalid", err)
	}
}
