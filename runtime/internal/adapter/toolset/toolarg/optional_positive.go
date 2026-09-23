// Package toolarg owns the one model-facing argument rule a JSON Schema cannot
// state: what an absent optional value means.
package toolarg

// OptionalInt resolves an absent optional integer to its default. The accepted
// range belongs to the argument's schema, which Scope validates against the
// frozen contract before the Tool executes — on the model's path and on direct
// diagnostic invocation alike — so a value that arrives here is already in
// range. Repeating the bound in Go would give it a second owner that can
// disagree with the one the model was shown.
func OptionalInt(value *int, fallback int) int {
	if value == nil {
		return fallback
	}
	return *value
}
