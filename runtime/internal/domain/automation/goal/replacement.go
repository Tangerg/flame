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
	replacement := Replacement{expected: expected, state: state.Clone()}
	if err := replacement.Validate(); err != nil {
		return Replacement{}, err
	}
	return replacement, nil
}

// ExpectedVersion returns the optional Goal version this state follows.
func (r Replacement) ExpectedVersion() Version { return r.expected }

// State returns an owned copy of the already-decided replacement state.
func (r Replacement) State() Goal { return r.state.Clone() }

// Validate proves that the state advances its expected version exactly once.
func (r Replacement) Validate() error {
	if err := r.expected.AdvancesTo(r.state); err != nil {
		return fmt.Errorf("goal: invalid replacement: %w", err)
	}
	return nil
}
