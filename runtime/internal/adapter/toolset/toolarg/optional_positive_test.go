package toolarg

import "testing"

func TestOptionalIntResolvesOnlyAbsence(t *testing.T) {
	if got := OptionalInt(nil, 8); got != 8 {
		t.Fatalf("absent = %d, want the default", got)
	}
	value := 3
	if got := OptionalInt(&value, 8); got != 3 {
		t.Fatalf("present = %d, want the supplied value", got)
	}
	// The schema bound is the argument's only owner, so a value that reached
	// this helper is already admitted — including one the helper never
	// inspected.
	edge := 0
	if got := OptionalInt(&edge, 8); got != 0 {
		t.Fatalf("admitted zero = %d, want it preserved", got)
	}
}
