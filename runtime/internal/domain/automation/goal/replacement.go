package goal

import "fmt"

// Replacement binds one already-decided Goal state to the exact optional
// version it advances. Persistence executes this decision without accepting a
// separately supplied CAS identity that could describe another Goal state.
type Replacement struct {
	expected Version
	state    Goal
}

// NewReplacement constructs one exact Goal version advance.
func NewReplacement(expected Version, state Goal) (Replacement, error) {
	replacement := Replacement{expected: expected, state: state}
	if err := expected.AdvancesTo(state); err != nil {
		return Replacement{}, fmt.Errorf("goal: invalid replacement: %w", err)
	}
	return replacement, nil
}

// ExpectedVersion returns the optional Goal version this state follows.
func (r Replacement) ExpectedVersion() Version { return r.expected }

// State returns the immutable, already-decided replacement state.
func (r Replacement) State() Goal { return r.state }

// IsZero reports whether no Goal replacement was constructed.
func (r Replacement) IsZero() bool { return r.state.IsZero() }
