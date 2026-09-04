package plan

import (
	"errors"
	"testing"
	"time"
)

func TestReplacementOwnsExactOptionalVersionAdvance(t *testing.T) {
	first, err := (Current{}).Replace([]Step{{Description: "first", Status: StatusPending}}, time.Unix(1, 0))
	if err != nil {
		t.Fatalf("create first state: %v", err)
	}
	initial, err := NewReplacement((Current{}).Version(), first)
	if err != nil {
		t.Fatalf("initial replacement: %v", err)
	}
	if !initial.ExpectedVersion().IsUnwritten() || initial.State().Revision() != 1 {
		t.Fatalf("initial replacement = %+v", initial)
	}

	current, err := CurrentOf(first)
	if err != nil {
		t.Fatal(err)
	}
	second, err := current.Replace([]Step{{Description: "second", Status: StatusCompleted}}, time.Unix(2, 0))
	if err != nil {
		t.Fatalf("create second state: %v", err)
	}
	next, err := NewReplacement(current.Version(), second)
	if err != nil {
		t.Fatalf("next replacement: %v", err)
	}
	if revision, committed := next.ExpectedVersion().Revision(); !committed || revision != 1 || next.State().Revision() != 2 {
		t.Fatalf("next replacement = %+v", next)
	}

	if _, err := NewReplacement((Current{}).Version(), second); !errors.Is(err, ErrInvalid) {
		t.Fatalf("skipped initial revision error = %v, want ErrInvalid", err)
	}
	if _, err := NewReplacement(current.Version(), first); !errors.Is(err, ErrInvalid) {
		t.Fatalf("non-advancing replacement error = %v, want ErrInvalid", err)
	}
}
