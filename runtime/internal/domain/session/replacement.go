package session

import (
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/exactint"
)

type Replacement struct {
	initial  bool
	expected Session
	state    Session
}

func InitialReplacement(state Session) (Replacement, error) {
	replacement := Replacement{initial: true, state: state}
	if err := replacement.Validate(); err != nil {
		return Replacement{}, err
	}
	return replacement, nil
}

func NextReplacement(expected, state Session) (Replacement, error) {
	replacement := Replacement{expected: expected, state: state}
	if err := replacement.Validate(); err != nil {
		return Replacement{}, err
	}
	return replacement, nil
}

func (r Replacement) Expected() (Session, bool) {
	return r.expected, !r.initial
}

func (r Replacement) State() Session { return r.state }

func (r Replacement) Validate() error {
	if r.state.ID().IsZero() {
		return fmt.Errorf("%w: replacement carries no Session", ErrInvalid)
	}
	if r.initial {
		firstRevision := exactint.First().Value()
		if !r.expected.ID().IsZero() || r.expected.Revision() != 0 {
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
