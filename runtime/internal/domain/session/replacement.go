package session

import (
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/exactint"
)

// Replacement binds one already-decided Session state to the exact aggregate
// revision it follows. A restored Session that does not yet exist is represented
// as an initial replacement whose expected revision is zero.
type Replacement struct {
	expected Session
	state    Session
}

// InitialReplacement constructs one initial Session write at revision one.
func InitialReplacement(state Session) (Replacement, error) {
	if state.IsZero() {
		return Replacement{}, fmt.Errorf("%w: replacement state is required", ErrInvalid)
	}
	if state.Revision() != exactint.First().Value() {
		return Replacement{}, fmt.Errorf("%w: initial replacement revision must be one", ErrInvalid)
	}
	return Replacement{state: state}, nil
}

// NextReplacement constructs one exact same-identity Session revision advance.
func NextReplacement(expected, state Session) (Replacement, error) {
	if expected.IsZero() || state.IsZero() {
		return Replacement{}, fmt.Errorf("%w: replacement requires expected and next sessions", ErrInvalid)
	}
	if expected.ID() != state.ID() {
		return Replacement{}, fmt.Errorf("%w: replacement changes Session identity from %q to %q", ErrInvalid, expected.ID(), state.ID())
	}
	if state.UpdatedAt().Before(expected.UpdatedAt()) {
		return Replacement{}, fmt.Errorf("%w: replacement moves Session update time backwards", ErrInvalid)
	}
	if err := exactint.Follows(expected.Revision(), state.Revision()); err != nil {
		return Replacement{}, fmt.Errorf("%w: replacement revision %d does not follow expected revision %d", ErrInvalid, state.Revision(), expected.Revision())
	}
	return Replacement{expected: expected, state: state}, nil
}

// ExpectedRevision returns zero for an initial replacement.
func (r Replacement) ExpectedRevision() uint64 { return r.expected.Revision() }

// State returns the complete already-decided replacement aggregate.
func (r Replacement) State() Session { return r.state }

// IsZero reports whether no replacement was constructed.
func (r Replacement) IsZero() bool { return r.state.IsZero() }
