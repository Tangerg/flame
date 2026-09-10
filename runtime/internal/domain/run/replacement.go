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

// ValidateDerivedBy proves state is exactly what expected becomes after taking
// on the progress state carries and then the transition the caller names, with
// no other fact rewritten. A replacement settles a Run that was still running,
// so the last metrics its executor committed arrive with the terminal decision;
// AdvanceProgress keeps them monotonic. The write paths choose the transition,
// but the value holding both aggregates owns the proof, so one replacement
// cannot be legal on its way through one path and illegal on the other.
func (r Replacement) ValidateDerivedBy(transition func(Run) (Run, error)) error {
	if err := r.Validate(); err != nil {
		return err
	}
	current, err := r.expected.AdvanceProgress(r.state.Metrics(), r.state.ContextTokens(), r.state.FinishedAt())
	if err != nil {
		return fmt.Errorf("run: replacement progress: %w", err)
	}
	derived, err := transition(current)
	if err != nil {
		return fmt.Errorf("run: replacement transition: %w", err)
	}
	if !derived.Equal(r.state) {
		return fmt.Errorf("run: replacement rewrites facts outside Run %q transition", r.expected.ID())
	}
	return nil
}

// Expected returns the complete aggregate the replacement was derived from.
func (r Replacement) Expected() Run { return r.expected }

// State returns the complete already-decided replacement aggregate.
func (r Replacement) State() Run { return r.state }
