package run

import "github.com/Tangerg/flame/packages/go-sdk/runoutcome"

type State string

const (
	Running  State = "running"
	Waiting  State = "waiting"
	Finished State = "finished"
)

func (s State) Valid() bool { return s == Running || s == Waiting || s == Finished }

func (s State) IsTerminal() bool { return s == Finished }

func (s State) Suspend() (State, bool) {
	if s == Running {
		return Waiting, true
	}
	return s, false
}

func (s State) Resume() (State, bool) {
	if s == Waiting {
		return Running, true
	}
	return s, false
}

func (s State) AdvanceProgress() bool { return s == Running || s == Waiting }

func (s State) Terminate(o Outcome) (State, bool) {
	if !o.valid() {
		return s, false
	}
	if s == Running || (s == Waiting && o == OutcomeCanceled) {
		return Finished, true
	}
	return s, false
}

func (s State) RecoverLost() (State, bool) {
	if s == Running || s == Waiting {
		return Finished, true
	}
	return s, false
}

type Outcome string

const (
	OutcomeCompleted Outcome = Outcome(runoutcome.Completed)
	OutcomeCanceled  Outcome = Outcome(runoutcome.Canceled)
	OutcomeTimedOut  Outcome = Outcome(runoutcome.TimedOut)
	OutcomeFailed    Outcome = Outcome(runoutcome.Failed)
	OutcomeLost      Outcome = Outcome(runoutcome.Lost)
)

func (o Outcome) valid() bool {
	switch o {
	case OutcomeCompleted, OutcomeCanceled, OutcomeTimedOut, OutcomeFailed,
		OutcomeLost:
		return true
	default:
		return false
	}
}

func ParseOutcome(s string) (Outcome, bool) {
	outcome := Outcome(s)
	if !outcome.valid() {
		return "", false
	}
	return outcome, true
}
