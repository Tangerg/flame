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
	replacement := Replacement{expected: expected, state: state.clone()}
	if err := replacement.Validate(); err != nil {
		return Replacement{}, err
	}
	return replacement, nil
}

// ExpectedVersion returns the optional Plan version this state follows.
func (r Replacement) ExpectedVersion() Version { return r.expected }

// State returns an owned copy of the already-decided replacement state.
func (r Replacement) State() State { return r.state.clone() }

// Validate proves that the state advances its expected version exactly once.
func (r Replacement) Validate() error {
	if err := r.expected.AdvancesTo(r.state); err != nil {
		return fmt.Errorf("plan: invalid replacement: %w", err)
	}
	return nil
}
