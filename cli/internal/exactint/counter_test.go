package exactint

import (
	"errors"
	"testing"
)

func TestCounterOwnsTheExactRangeAndCheckedAdvance(t *testing.T) {
	t.Parallel()

	zero, err := Restore(0)
	if err != nil || !zero.IsZero() {
		t.Fatalf("Restore(0) = %d, %v", zero.Value(), err)
	}
	first, err := zero.Next()
	if err != nil || first != First() {
		t.Fatalf("zero.Next() = %d, %v", first.Value(), err)
	}
	last, err := Restore(Maximum)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := last.Next(); !errors.Is(err, ErrExhausted) {
		t.Fatalf("last.Next() error = %v", err)
	}
	if _, err := first.Advance(Maximum); !errors.Is(err, ErrExhausted) {
		t.Fatalf("first.Advance(Maximum) error = %v", err)
	}
	if _, err := Restore(Maximum + 1); !errors.Is(err, ErrOutOfRange) {
		t.Fatalf("Restore(Maximum+1) error = %v", err)
	}
	if err := Follows(Maximum-1, Maximum); err != nil {
		t.Fatalf("Follows(Maximum-1, Maximum): %v", err)
	}
	if err := Follows(Maximum, 0); !errors.Is(err, ErrExhausted) {
		t.Fatalf("Follows(Maximum, 0) error = %v", err)
	}
	if err := Follows(7, 9); !errors.Is(err, ErrNotSuccessor) {
		t.Fatalf("Follows(7, 9) error = %v", err)
	}
}
