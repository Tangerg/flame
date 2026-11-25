package run

import (
	"slices"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/packages/go-sdk/runoutcome"
)

type State string

const (
	Running  State = "running"
	Waiting  State = "waiting"
	Finished State = "finished"
)

func (s State) Valid() bool { return s.IsOpen() || s.IsTerminal() }

func (s State) IsTerminal() bool { return s == Finished }

func (s State) IsOpen() bool { return s == Running || s == Waiting }

type Lifecycle interface {
	state() State
}

type ActiveSegment struct {
	ID identity.SegmentID
}

type Parked struct{}

type Termination struct {
	Ending            Ending
	UnresolvedEffects []UnresolvedEffect
	FinishedAt        time.Time
	MessageMark       MessageMark
}

func (ActiveSegment) state() State { return Running }
func (Parked) state() State        { return Waiting }
func (Termination) state() State   { return Finished }

func (t Termination) equal(other Termination) bool {
	return t.Ending == other.Ending && t.FinishedAt.Equal(other.FinishedAt) &&
		t.MessageMark == other.MessageMark && slices.Equal(t.UnresolvedEffects, other.UnresolvedEffects)
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
