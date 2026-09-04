package run

import (
	"errors"
	"fmt"
)

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

// Validate proves both aggregates are valid and retain one Run identity.
func (r Replacement) Validate() error {
	if err := r.expected.Validate(); err != nil {
		return fmt.Errorf("run: replacement expected state: %w", err)
	}
	if err := r.state.Validate(); err != nil {
		return fmt.Errorf("run: replacement state: %w", err)
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
