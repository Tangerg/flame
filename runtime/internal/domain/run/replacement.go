package run

import "errors"

// Replacement binds an already-decided Run state to the exact aggregate it
// was derived from. Application write-sets use it when persistence must reject
// a different current state rather than recomputing a transition from storage.
type Replacement struct {
	expected Run
	state    Run
}

// NewReplacement constructs one exact Run aggregate replacement.
func NewReplacement(expected, state Run) (Replacement, error) {
	if expected.IsZero() || state.IsZero() {
		return Replacement{}, errors.New("run: replacement requires expected and next runs")
	}
	if expected.ID() != state.ID() || expected.SessionID() != state.SessionID() {
		return Replacement{}, errors.New("run: replacement changes Run identity")
	}
	return Replacement{expected: expected, state: state}, nil
}

// IsZero reports whether no replacement was constructed.
func (r Replacement) IsZero() bool { return r.state.IsZero() }

// Expected returns the complete aggregate the replacement was derived from.
func (r Replacement) Expected() Run { return r.expected }

// State returns the complete already-decided replacement aggregate.
func (r Replacement) State() Run { return r.state }
