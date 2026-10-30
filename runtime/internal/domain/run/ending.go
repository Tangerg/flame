package run

import (
	"errors"
	"fmt"
	"strings"
)

// Ending is how a finished Run ended. The variant alone decides the Run's
// Outcome and carries exactly the facts that outcome explains.
type Ending interface {
	Outcome() Outcome
	validate() error
}

// Completed is a Run whose model returned a stop-marker normally.
type Completed struct{}

// Canceled is a Run its caller canceled; Detail is the stated reason.
type Canceled struct{ Detail string }

// Failed is a Run that aborted on a classified failure.
type Failed struct{ Failure Failure }

// TimedOut is a Run that exceeded its governing deadline.
type TimedOut struct{ Detail string }

// Lost is a Run whose real outcome cannot be established.
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

func validateEnding(ending Ending) error {
	if ending == nil {
		return errors.New("run: terminal Run has no ending")
	}
	if !ending.Outcome().valid() {
		return fmt.Errorf("run: unknown outcome %q", ending.Outcome())
	}
	return ending.validate()
}
