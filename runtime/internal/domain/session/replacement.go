package session

import (
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/exactint"
)

// Replacement binds one already-decided Session state to the exact aggregate
// revision it follows. A restored Session that does not yet exist is represented
// as an initial replacement whose expected revision is zero.
type Replacement struct {
	initial  bool
	expected Session
	state    Session
}

// InitialReplacement constructs one initial Session write at revision one.
func InitialReplacement(state Session) (Replacement, error) {
	replacement := Replacement{initial: true, state: state}
	if err := replacement.Validate(); err != nil {
		return Replacement{}, err
	}
	return replacement, nil
}

// NextReplacement constructs one exact same-identity Session revision advance.
func NextReplacement(expected, state Session) (Replacement, error) {
	replacement := Replacement{expected: expected, state: state}
	if err := replacement.Validate(); err != nil {
		return Replacement{}, err
	}
	return replacement, nil
}

// ExpectedRevision returns zero for an initial replacement.
func (r Replacement) ExpectedRevision() uint64 {
	if r.initial {
		return 0
	}
	return r.expected.Revision()
}

// State returns the complete already-decided replacement aggregate.
func (r Replacement) State() Session { return r.state }

// Validate proves that r is either one initial aggregate or one exact
// same-identity monotonic replacement.
func (r Replacement) Validate() error {
	if err := r.state.Validate(); err != nil {
		return fmt.Errorf("%w: replacement state: %v", ErrInvalid, err)
	}
	if r.initial {
		firstRevision := exactint.First().Value()
		if r.expected.ID() != "" || r.expected.Revision() != 0 {
			return fmt.Errorf("%w: initial replacement has an expected Session", ErrInvalid)
		}
		if r.state.Revision() != firstRevision {
			return fmt.Errorf(
				"%w: initial replacement revision is %d, want %d",
				ErrInvalid,
				r.state.Revision(),
				firstRevision,
			)
		}
		return nil
	}
	if err := r.expected.Validate(); err != nil {
		return fmt.Errorf("%w: replacement expected Session: %v", ErrInvalid, err)
	}
	if r.expected.ID() != r.state.ID() {
		return fmt.Errorf(
			"%w: replacement changes Session identity from %q to %q",
			ErrInvalid,
			r.expected.ID(),
			r.state.ID(),
		)
	}
	if r.state.UpdatedAt().Before(r.expected.UpdatedAt()) {
		return fmt.Errorf("%w: replacement moves Session update time backwards", ErrInvalid)
	}
	if err := exactint.Follows(r.expected.Revision(), r.state.Revision()); err != nil {
		return fmt.Errorf(
			"%w: replacement revision %d does not follow expected revision %d",
			ErrInvalid,
			r.state.Revision(),
			r.expected.Revision(),
		)
	}
	return nil
}
