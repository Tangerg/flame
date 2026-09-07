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
	if expected.IsZero() || state.IsZero() {
		return Replacement{}, fmt.Errorf("transcript: replacement requires expected and next items")
	}
	if expected.ID() != state.ID() || expected.SessionID() != state.SessionID() || expected.RunID() != state.RunID() {
		return Replacement{}, fmt.Errorf("%w: replacement changes Item %q ownership", ErrIdentityConflict, expected.ID())
	}
	return Replacement{expected: expected, state: state}, nil
}

// IsZero reports whether no replacement was constructed.
func (r Replacement) IsZero() bool { return r.state.IsZero() }

// Expected returns the complete Item the replacement was derived from.
func (r Replacement) Expected() Item { return r.expected }

// State returns the complete already-decided replacement Item.
func (r Replacement) State() Item { return r.state }
