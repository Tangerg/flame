package run

import "errors"

// Replacement binds a decided Run state to the exact aggregate it was derived
// from. It is built only by applying a transition to that aggregate, so its
// legality is settled at construction; persistence fences the expected state
// and writes the decided one without recomputing the transition.
type Replacement struct {
	expected Run
	state    Run
}

// Replace derives one Replacement by applying transition to expected.
func Replace(expected Run, transition func(Run) (Run, error)) (Replacement, error) {
	if expected.ID() == "" {
		return Replacement{}, errors.New("run: replacement carries no Run")
	}
	state, err := transition(expected)
	if err != nil {
		return Replacement{}, err
	}
	return Replacement{expected: expected, state: state}.checked()
}

// Then extends the decided state with a further transition while keeping the
// aggregate the replacement was derived from.
func (r Replacement) Then(transition func(Run) (Run, error)) (Replacement, error) {
	if err := r.Validate(); err != nil {
		return Replacement{}, err
	}
	state, err := transition(r.state)
	if err != nil {
		return Replacement{}, err
	}
	return Replacement{expected: r.expected, state: state}.checked()
}

func (r Replacement) checked() (Replacement, error) {
	if err := r.Validate(); err != nil {
		return Replacement{}, err
	}
	return r, nil
}

// Validate rejects the zero value and a transition that changed identity.
func (r Replacement) Validate() error {
	if r.expected.ID() == "" {
		return errors.New("run: replacement carries no Run")
	}
	if r.expected.ID() != r.state.ID() || r.expected.SessionID() != r.state.SessionID() {
		return errors.New("run: replacement changes Run identity")
	}
	return nil
}

// Expected returns the complete aggregate the replacement was derived from.
func (r Replacement) Expected() Run { return r.expected }

// State returns the complete already-decided replacement aggregate.
func (r Replacement) State() Run { return r.state }
