package run

import "github.com/Tangerg/flame/packages/go-sdk/runoutcome"

// State is the lifecycle position of a Run and the single authority for its
// legal transitions. Why a Finished Run ended is its [Outcome], never a second
// state value; persistence and the protocol store these same three spellings.
//
//	Running ──Suspend──▶ Waiting
//	   │  ▲                   │
//	   │  └──────Resume───────┘
//	Terminate(o)       Terminate(Canceled)
//	   ▼                      ▼
//	          Finished
//
// Root admission ("one non-terminal root Run per Session") keys on
// [State.IsTerminal]; descendants share their root tree's admission.
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

// AdvanceProgress reports whether a model-response boundary may be committed:
// both open positions accumulate work.
func (s State) AdvanceProgress() bool { return s == Running || s == Waiting }

// Terminate is legal from Running for any outcome and from Waiting only for
// [OutcomeCanceled]: a parked Run reaches any other ending only after resuming.
func (s State) Terminate(o Outcome) (State, bool) {
	if !o.valid() {
		return s, false
	}
	if s == Running || (s == Waiting && o == OutcomeCanceled) {
		return Finished, true
	}
	return s, false
}

// RecoverLost ends a Run whose executor disappeared without a resumable
// interrupt, including an orphaned Waiting Run.
func (s State) RecoverLost() (State, bool) {
	if s == Running || s == Waiting {
		return Finished, true
	}
	return s, false
}

// Outcome is why a Run reached [Finished]. Persistence and presentation
// both project from this single terminal-reason taxonomy.
//
// An interrupt is deliberately NOT an Outcome: parking is the [Waiting]
// state, not a terminal reason. A run that ends while parked ends via
// [OutcomeCanceled].
type Outcome string

const (
	// OutcomeCompleted — the model returned a stop-marker normally.
	OutcomeCompleted Outcome = Outcome(runoutcome.Completed)
	// OutcomeCanceled — the caller canceled, or the context was canceled.
	OutcomeCanceled Outcome = Outcome(runoutcome.Canceled)
	// OutcomeTimedOut — the Run exceeded its governing deadline. This is not a
	// generic failure: callers may apply a distinct retry and alerting policy.
	OutcomeTimedOut Outcome = Outcome(runoutcome.TimedOut)
	// OutcomeFailed — the run aborted on an error.
	OutcomeFailed Outcome = Outcome(runoutcome.Failed)
	// OutcomeLost — the Run's real outcome cannot be established: recovery
	// found nothing that can continue it, or its effects have no provable result.
	OutcomeLost Outcome = Outcome(runoutcome.Lost)
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

// ParseOutcome maps an outcome's [Outcome.String] form back to the value,
// reporting false for anything else. It sits next to String because they are one
// mapping read in two directions: a durable record must come back as the same
// terminal reason it was written as, and a second hand-written table somewhere
// downstream would be free to disagree with this one.
func ParseOutcome(s string) (Outcome, bool) {
	outcome := Outcome(s)
	if !outcome.valid() {
		return "", false
	}
	return outcome, true
}
