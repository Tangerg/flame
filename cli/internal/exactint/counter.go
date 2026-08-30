// Package exactint owns monotonic non-negative counters whose identity crosses
// JSON boundaries commonly decoded through IEEE-754 binary64 values. Product
// concepts decide whether zero is legal; this package owns only exactness and
// checked advancement.
package exactint

import "errors"

// Maximum is the greatest integer every IEEE-754 binary64 consumer represents
// exactly. JSON has no numeric precision limit, but JavaScript and common JSON
// implementations decode numbers through binary64.
const Maximum uint64 = 1<<53 - 1

var (
	ErrOutOfRange   = errors.New("exact integer counter is outside the supported range")
	ErrExhausted    = errors.New("exact integer counter is exhausted")
	ErrNotSuccessor = errors.New("exact integer counter does not advance exactly once")
)

// Counter is a value in the inclusive range zero through [Maximum].
type Counter struct{ value uint64 }

// Restore reconstructs a Counter while rejecting an inexact boundary value.
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

// IsZero reports whether no value has been committed.
func (c Counter) IsZero() bool { return c.value == 0 }

// Next returns the next exact value without wrapping.
func (c Counter) Next() (Counter, error) { return c.Advance(1) }

// Advance reserves a fixed number of monotonic changes without iterating.
func (c Counter) Advance(changes uint64) (Counter, error) {
	if c.value > Maximum {
		return Counter{}, ErrOutOfRange
	}
	if changes > Maximum-c.value {
		return Counter{}, ErrExhausted
	}
	return Counter{value: c.value + changes}, nil
}

// Follows verifies that next is the exact successor of previous.
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
