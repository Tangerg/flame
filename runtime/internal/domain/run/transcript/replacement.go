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

// Validate proves the two aggregates were constructed and retain one Item
// identity. Their legality is settled by the constructors that produced them;
// only the zero value can reach here unbuilt.
func (r Replacement) Validate() error {
	if r.expected.ID() == "" {
		return fmt.Errorf("%w: replacement carries no Item", ErrIdentityConflict)
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
