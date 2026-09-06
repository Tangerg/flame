package schedule

import (
	"errors"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/exactint"
)

// Replacement binds one management-edited Schedule to the exact aggregate it
// follows. Operational cursor claims and accepted Run records use their own
// domain values and cannot be smuggled through this management write.
type Replacement struct {
	expected Schedule
	state    Schedule
}

// NewReplacement constructs one exact Schedule management replacement.
func NewReplacement(expected, state Schedule) (Replacement, error) {
	replacement := Replacement{expected: expected, state: state}
	if err := replacement.Validate(); err != nil {
		return Replacement{}, err
	}
	return replacement, nil
}

// ExpectedRevision returns the exact revision this replacement follows.
func (r Replacement) ExpectedRevision() uint64 { return r.expected.Revision() }

// State returns the already-decided replacement Schedule.
func (r Replacement) State() Schedule { return r.state }

// Validate proves that a management edit preserves identity and operational
// lifecycle facts while advancing exactly one revision.
func (r Replacement) Validate() error {
	if err := r.expected.Validate(); err != nil {
		return fmt.Errorf("schedule: replacement expected state: %w", err)
	}
	if err := r.state.Validate(); err != nil {
		return fmt.Errorf("schedule: replacement state: %w", err)
	}
	if r.expected.ID() != r.state.ID() {
		return fmt.Errorf("schedule: replacement changes identity from %q to %q", r.expected.ID(), r.state.ID())
	}
	if !r.expected.CreatedAt().Equal(r.state.CreatedAt()) {
		return errors.New("schedule: replacement changes schedule creation time")
	}
	if !r.expected.LastRunAt().Equal(r.state.LastRunAt()) {
		return errors.New("schedule: replacement changes the accepted run cursor")
	}
	if err := exactint.Follows(r.expected.Revision(), r.state.Revision()); err != nil {
		return fmt.Errorf(
			"schedule: replacement revision %d does not follow expected revision %d: %w",
			r.state.Revision(),
			r.expected.Revision(),
			ErrRevisionConflict,
		)
	}
	return nil
}
