package plan

import "fmt"

// Replacement binds one already-decided Plan state to the exact optional
// version it advances. Persistence executes this decision without reconstructing
// the revision relationship from separate arguments.
type Replacement struct {
	expected Version
	state    State
}

// NewReplacement constructs one exact Plan version advance.
func NewReplacement(expected Version, state State) (Replacement, error) {
	replacement := Replacement{expected: expected, state: state}
	if err := expected.AdvancesTo(state); err != nil {
		return Replacement{}, fmt.Errorf("plan: invalid replacement: %w", err)
	}
	return replacement, nil
}

// ExpectedVersion returns the optional Plan version this state follows.
func (r Replacement) ExpectedVersion() Version { return r.expected }

// State returns the immutable, already-decided replacement state.
func (r Replacement) State() State { return r.state }

// IsZero reports whether no Plan replacement was constructed.
func (r Replacement) IsZero() bool { return r.state.IsZero() }
