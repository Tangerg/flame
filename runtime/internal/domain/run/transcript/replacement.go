package transcript

import "fmt"

// Replacement binds an already-decided Item state to the exact aggregate it
// was derived from. Persistence uses it to reject a different current Item
// rather than overwriting a racing transcript transition.
type Replacement struct {
	expected Item
	state    Item
}

// NewReplacement constructs one exact Item aggregate replacement.
func NewReplacement(expected, state Item) (Replacement, error) {
	replacement := Replacement{expected: expected, state: state}
	if err := replacement.Validate(); err != nil {
		return Replacement{}, err
	}
	return replacement, nil
}

// Validate proves both aggregates are valid and retain one Item identity.
func (r Replacement) Validate() error {
	if err := r.expected.Validate(); err != nil {
		return fmt.Errorf("transcript: replacement expected Item: %w", err)
	}
	if err := r.state.Validate(); err != nil {
		return fmt.Errorf("transcript: replacement state Item: %w", err)
	}
	if r.expected.ID() != r.state.ID() ||
		r.expected.SessionID() != r.state.SessionID() ||
		r.expected.RunID() != r.state.RunID() {
		return fmt.Errorf(
			"%w: replacement changes Item %q ownership",
			ErrIdentityConflict,
			r.expected.ID(),
		)
	}
	return nil
}

// Expected returns the complete Item the replacement was derived from.
func (r Replacement) Expected() Item { return r.expected }

// State returns the complete already-decided replacement Item.
func (r Replacement) State() Item { return r.state }
