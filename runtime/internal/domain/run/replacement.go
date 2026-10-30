package run

import (
	"errors"
	"time"
)

// Replacement binds a decided Run state to the exact aggregate it was derived
// from. It advances only through the Run's own named transitions, so its
// legality is settled at construction; persistence fences the expected state
// and writes the decided one without recomputing the transition.
type Replacement struct {
	expected Run
	state    Run
}

// Replace starts a Replacement of r that has not moved yet.
func (r Run) Replace() Replacement {
	return Replacement{expected: r, state: r}
}

func (r Replacement) advance(state Run, err error) (Replacement, error) {
	if err != nil {
		return Replacement{}, err
	}
	return Replacement{expected: r.expected, state: state}, nil
}

func (r Replacement) AdvanceProgress(metrics Metrics, contextTokens int64, updatedAt time.Time) (Replacement, error) {
	return r.advance(r.state.AdvanceProgress(metrics, contextTokens, updatedAt))
}

func (r Replacement) Resume(segmentID string, resumedAt time.Time) (Replacement, error) {
	return r.advance(r.state.Resume(segmentID, resumedAt))
}

func (r Replacement) CancelWaiting(detail string, finishedAt time.Time, messageMark MessageMark) (Replacement, error) {
	return r.advance(r.state.CancelWaiting(detail, finishedAt, messageMark))
}

func (r Replacement) RecoverLost(failure Failure, finishedAt time.Time, messageMark MessageMark) (Replacement, error) {
	return r.advance(r.state.RecoverLost(failure, finishedAt, messageMark))
}

func (r Replacement) WithMessageMark(messageMark int) (Replacement, error) {
	return r.advance(r.state.WithMessageMark(messageMark))
}

// Validate rejects a Replacement that was never started from a Run.
func (r Replacement) Validate() error {
	if r.expected.ID() == "" {
		return errors.New("run: replacement carries no Run")
	}
	return nil
}

// Expected returns the complete aggregate the replacement was derived from.
func (r Replacement) Expected() Run { return r.expected }

// State returns the complete already-decided replacement aggregate.
func (r Replacement) State() Run { return r.state }
