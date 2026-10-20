// Package exactint owns monotonic non-negative counters that must retain their
// identity when they cross boundaries represented by IEEE-754 doubles. It is a
// transport-neutral primitive shared by Domain, persistence, and public-wire
// adapters; product concepts such as Session revisions remain in their owners.
package exactint

import "errors"

// Maximum is the greatest integer that every IEEE-754 binary64 consumer can
// represent exactly. JSON itself has no numeric precision limit, but JavaScript
// and several common JSON implementations decode numbers through binary64.
const Maximum = 1<<53 - 1

var (
	// ErrOutOfRange reports reconstruction of a counter outside the exact range.
	ErrOutOfRange = errors.New("exact integer counter is outside the supported range")
	// ErrExhausted reports an attempted advance beyond [Maximum].
	ErrExhausted = errors.New("exact integer counter is exhausted")
	// ErrNotSuccessor reports two values that are not one monotonic step apart.
	ErrNotSuccessor = errors.New("exact integer counter does not advance exactly once")
)

// Counter is a monotonic value in the inclusive range zero through [Maximum].
// Zero is useful for an unwritten/unassigned state; domain aggregates decide
// whether zero is legal for their own lifecycle.
type Counter struct{ value uint64 }

// Restore reconstructs a Counter while rejecting an inexact persisted value.
func Restore(value uint64) (Counter, error) {
	if value > Maximum {
		return Counter{}, ErrOutOfRange
	}
	return Counter{value: value}, nil
}

// First returns the first committed counter value.
func First() Counter { return Counter{value: 1} }

// Value returns the exact scalar representation.
func (c Counter) Value() uint64 { return c.value }

// IsZero reports whether no value has been assigned yet.
func (c Counter) IsZero() bool { return c.value == 0 }

// Next returns the next exact value. It never wraps.
func (c Counter) Next() (Counter, error) {
	if c.value > Maximum {
		return Counter{}, ErrOutOfRange
	}
	if c.value == Maximum {
		return Counter{}, ErrExhausted
	}
	return Counter{value: c.value + 1}, nil
}

// Follows verifies that next is the exact successor of previous. Both values
// are range-checked and the relation is evaluated without overflow arithmetic.
func Follows(previous, next uint64) error {
	current, err := Restore(previous)
	if err != nil {
		return err
	}
	want, err := current.Next()
	if err != nil {
		return err
	}
	actual, err := Restore(next)
	if err != nil {
		return err
	}
	if actual != want {
		return ErrNotSuccessor
	}
	return nil
}
