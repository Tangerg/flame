package transcript

import "fmt"

// Replacement binds a decided Item state to the exact aggregate it was
// derived from. It is built only by applying a transition to that Item, so its
// legality is settled at construction; persistence rejects a different current
// Item rather than overwriting a racing transcript transition.
type Replacement struct {
	expected Item
	state    Item
}

// Replace derives one Replacement by applying transition to expected.
func Replace(expected Item, transition func(Item) (Item, error)) (Replacement, error) {
	if expected.ID() == "" {
		return Replacement{}, fmt.Errorf("%w: replacement carries no Item", ErrIdentityConflict)
	}
	state, err := transition(expected)
	if err != nil {
		return Replacement{}, err
	}
	replacement := Replacement{expected: expected, state: state}
	if err := replacement.validate(); err != nil {
		return Replacement{}, err
	}
	return replacement, nil
}

// validate rejects the zero value and a transition that changed ownership.
func (r Replacement) validate() error {
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
