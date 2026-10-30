package transcript

// Replacement binds a decided Item state to the exact Item it was derived
// from. Only the Item's named transitions build one, so its legality is
// settled at construction; persistence rejects a different current Item rather
// than overwriting a racing transcript transition.
type Replacement struct {
	expected Item
	state    Item
}

func (i Item) replacedBy(state Item, err error) (Replacement, error) {
	if err != nil {
		return Replacement{}, err
	}
	return Replacement{expected: i, state: state}, nil
}

// Expected returns the complete Item the replacement was derived from.
func (r Replacement) Expected() Item { return r.expected }

// State returns the complete already-decided replacement Item.
func (r Replacement) State() Item { return r.state }
