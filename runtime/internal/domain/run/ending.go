package run

import (
	"errors"
	"strings"
)

type Ending interface {
	Outcome() Outcome
	validate() error
}

type Completed struct{}

type Canceled struct{ Detail string }

type Failed struct{ Failure Failure }

type TimedOut struct{ Detail string }

type Lost struct{ Loss Loss }

func (Completed) Outcome() Outcome { return OutcomeCompleted }
func (Canceled) Outcome() Outcome  { return OutcomeCanceled }
func (Failed) Outcome() Outcome    { return OutcomeFailed }
func (TimedOut) Outcome() Outcome  { return OutcomeTimedOut }
func (Lost) Outcome() Outcome      { return OutcomeLost }

func (Completed) validate() error { return nil }

func (c Canceled) validate() error { return validateEndingDetail(c.Detail) }

func (f Failed) validate() error { return f.Failure.Validate() }

func (t TimedOut) validate() error { return validateEndingDetail(t.Detail) }

func (l Lost) validate() error { return l.Loss.Validate() }

func validateEndingDetail(detail string) error {
	if detail != strings.TrimSpace(detail) {
		return errors.New("run: ending detail has surrounding whitespace")
	}
	return nil
}

func ValidateEnding(ending Ending) error {
	switch ending.(type) {
	case nil:
		return errors.New("run: terminal Run has no ending")
	case Completed, Canceled, Failed, TimedOut, Lost:
		return ending.validate()
	default:
		// Pointer variants would let callers change committed facts and bypass
		// the value variants consumed by the aggregate's queries.
		return errors.New("run: ending must be an immutable value variant")
	}
}
