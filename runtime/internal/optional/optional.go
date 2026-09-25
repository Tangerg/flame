// Package optional owns what an absent optional value means, for the two kinds
// of caller that ask: a model-supplied argument whose bounds a JSON Schema
// already states, and an owner-supplied policy override that nothing else
// validates.
package optional

import "fmt"

// Value resolves an absent optional value to its default. An argument that
// reached here was already validated against the frozen contract Scope checks
// before a Tool executes — on the model's path and on direct diagnostic
// invocation alike — so repeating a bound in Go would give it a second owner
// that can disagree with the one the model was shown.
func Value[T any](value *T, fallback T) T {
	if value == nil {
		return fallback
	}
	return *value
}

// Clone copies what an optional value points at, so an absent one stays absent
// and a present one stops sharing its storage with the value it came from. The
// copy is shallow: it isolates the pointer, which is what makes an aggregate's
// optional field safe to hand out, and says nothing about any reference the
// pointed-at value holds itself.
func Clone[T any](value *T) *T {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

// Positive resolves an absent policy override to its default and rejects a
// non-positive one. No schema stands between configuration and this value, and
// a zero limit is not a smaller limit — it is a policy that admits nothing.
// field names the setting in the error, which is the only part a caller varies.
func Positive[T ~int | ~int64](value *T, fallback T, field string) (T, error) {
	var zero T
	if fallback <= zero {
		return zero, fmt.Errorf("%s default must be positive", field)
	}
	if value == nil {
		return fallback, nil
	}
	if *value <= zero {
		return zero, fmt.Errorf("%s must be positive", field)
	}
	return *value, nil
}
