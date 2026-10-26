package optional

import (
	"testing"
	"time"
)

// TestPositiveRefusesAnUnusableDefault covers the arm no policy value can
// reach: the fallback is a composition-root constant, so only the rule itself
// can refuse one that admits nothing.
func TestPositiveRefusesAnUnusableDefault(t *testing.T) {
	if _, err := Positive[int](nil, 0, "tool concurrency"); err == nil {
		t.Fatal("a zero signed default was accepted")
	}
	if _, err := Positive[time.Duration](nil, 0, "poll interval"); err == nil {
		t.Fatal("a zero duration default was accepted")
	}
	value, err := Positive[int](nil, 3, "tool concurrency")
	if err != nil || value != 3 {
		t.Fatalf("Positive(absent, 3) = (%d, %v)", value, err)
	}
	if _, err := Positive(new(int), 3, "tool concurrency"); err == nil {
		t.Fatal("a zero override was accepted")
	}
}

// TestValueKeepsAnInRangeOverride holds the other half of the contract: a value
// that reached here was already admitted by its schema, so it is returned as
// given, including one a policy check would have refused.
func TestValueKeepsAnInRangeOverride(t *testing.T) {
	if got := Value[int](nil, 7); got != 7 {
		t.Fatalf("Value(absent, 7) = %d", got)
	}
	supplied := 3
	if got := Value(&supplied, 7); got != 3 {
		t.Fatalf("Value(present 3, 7) = %d", got)
	}
	zero := 0
	if got := Value(&zero, 7); got != 0 {
		t.Fatalf("Value(present zero, 7) = %d", got)
	}
}
