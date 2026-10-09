package goal

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
)

func TestGoalTimesRemainExactlyRepresentable(t *testing.T) {
	for _, at := range []time.Time{time.Unix(0, math.MinInt64).UTC(), time.Unix(0, 0).UTC(), time.Unix(0, math.MaxInt64).UTC()} {
		value, err := New("session", "objective", modelref.Selection{}, run.Capabilities{}, "incarnation", at)
		if err != nil || !value.CreatedAt().Equal(at) || !value.UpdatedAt().Equal(at) {
			t.Fatalf("New at %s = %+v, %v", at, value.Snapshot(), err)
		}
	}
	for _, at := range []time.Time{time.Unix(0, math.MinInt64).Add(-time.Nanosecond), time.Unix(0, math.MaxInt64).Add(time.Nanosecond)} {
		if _, err := New("session", "objective", modelref.Selection{}, run.Capabilities{}, "incarnation", at); !errors.Is(err, ErrInvalid) {
			t.Errorf("New at %s = %v, want ErrInvalid", at, err)
		}
	}
	value, err := New("session", "objective", modelref.Selection{}, run.Capabilities{}, "incarnation", time.Unix(0, math.MaxInt64).UTC())
	if err != nil {
		t.Fatal(err)
	}
	outside := value.UpdatedAt().Add(time.Nanosecond)
	if _, err := value.Complete(outside); !errors.Is(err, ErrInvalid) {
		t.Errorf("Complete past exact range = %v, want ErrInvalid", err)
	}
	if _, err := value.ReviseObjective("changed", "next_incarnation", outside); !errors.Is(err, ErrInvalid) {
		t.Errorf("ReviseObjective past exact range = %v, want ErrInvalid", err)
	}
	snapshot := value.Snapshot()
	snapshot.UpdatedAt = outside
	if _, err := Restore(snapshot); !errors.Is(err, ErrInvalid) {
		t.Errorf("Restore past exact range = %v, want ErrInvalid", err)
	}
}
