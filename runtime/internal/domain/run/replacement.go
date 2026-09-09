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
	replacement := Replacement{expected: expected, state: state}
	if err := replacement.Validate(); err != nil {
		return Replacement{}, err
	}
	return replacement, nil
}

// Validate proves both aggregates were constructed and retain one Run
// identity. Their legality is settled by the constructors and transitions that
// produced them; only the zero value can reach here unbuilt.
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
