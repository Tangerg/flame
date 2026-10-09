package run

import "errors"

// Replacement binds a decided Run state to the exact aggregate it was derived
// from. It is built only by applying a transition to that aggregate, so its
// legality is settled at construction; persistence fences the expected state
// and writes the decided one without recomputing the transition.
type Replacement struct {
	expected Run
	state    Run
}

// Replace derives one Replacement by applying transition to expected.
func Replace(expected Run, transition func(Run) (Run, error)) (Replacement, error) {
	if expected.ID() == "" {
		return Replacement{}, errors.New("run: replacement carries no Run")
	}
	state, err := transition(expected)
	if err != nil {
		return Replacement{}, err
	}
	replacement, err := (Replacement{expected: expected, state: state}).checked()
	if err != nil {
		return Replacement{}, err
	}
	if err := replacement.validateTransition(); err != nil {
		return Replacement{}, err
	}
	return replacement, nil
}

// Each step must be reproducible by the aggregate's own transitions. A chain
// may span several lifecycle positions, so this proof belongs to Replace and
// Then rather than comparing only the chain's first and last states.
func (r Replacement) validateTransition() error {
	if r.expected.Equal(r.state) {
		return nil
	}
	decided := r.expected
	var err error
	if decided.state.IsTerminal() {
		count, known := r.state.messageMark.Count()
		if !known {
			return errors.New("run: replacement clears settled message watermark")
		}
		decided, err = decided.WithMessageMark(count)
	} else {
		decided, err = decided.AdvanceProgress(r.state.metrics, r.state.contextTokens, r.state.updatedAt)
		if err != nil {
			return err
		}
		switch {
		case r.state.state.IsTerminal():
			outcome, present := r.state.Outcome()
			if !present {
				return errors.New("run: replacement has no terminal outcome")
			}
			if outcome == OutcomeLost && decided.state == Waiting {
				failure, failureErr := r.state.LostFailure()
				if failureErr != nil {
					return failureErr
				}
				decided, err = decided.RecoverLost(failure, r.state.finishedAt, r.state.messageMark)
			} else {
				decided, err = decided.Terminate(Termination{
					Outcome: outcome, UnresolvedEffects: r.state.unresolvedEffects,
					Detail: r.state.detail, Failure: r.state.failure,
					FinishedAt: r.state.finishedAt, MessageMark: r.state.messageMark,
				})
			}
		case decided.state != r.state.state && r.state.state == Waiting:
			decided, err = decided.Suspend(r.state.updatedAt)
		case decided.state != r.state.state && r.state.state == Running:
			decided, err = decided.Resume(r.state.activeSegmentID, r.state.updatedAt)
		}
	}
	if err != nil {
		return err
	}
	if !decided.Equal(r.state) {
		return errors.New("run: replacement rewrites facts outside a Run transition")
	}
	return nil
}

// Then extends the decided state with a further transition while keeping the
// aggregate the replacement was derived from. The intermediate state remains
// the owner of its settled facts while the replacement is extended.
func (r Replacement) Then(transition func(Run) (Run, error)) (Replacement, error) {
	if err := r.Validate(); err != nil {
		return Replacement{}, err
	}
	next, err := Replace(r.state, transition)
	if err != nil {
		return Replacement{}, err
	}
	return Replacement{expected: r.expected, state: next.state}.checked()
}

func (r Replacement) checked() (Replacement, error) {
	if err := r.Validate(); err != nil {
		return Replacement{}, err
	}
	return r, nil
}

// Validate fences admission facts, cumulative accounting, and settled outcomes.
// Restore reconstructs historical values; it cannot authorize a live replacement.
func (r Replacement) Validate() error {
	if r.expected.ID() == "" {
		return errors.New("run: replacement carries no Run")
	}
	if r.expected.ID() != r.state.ID() || r.expected.SessionID() != r.state.SessionID() {
		return errors.New("run: replacement changes Run identity")
	}
	if r.expected.lineage != r.state.lineage ||
		!r.expected.modelSelection.Equal(r.state.modelSelection) ||
		r.expected.goalIncarnationID != r.state.goalIncarnationID ||
		!r.expected.capabilities.Equal(r.state.capabilities) ||
		!r.expected.createdAt.Equal(r.state.createdAt) {
		return errors.New("run: replacement changes Run admission")
	}
	if err := r.state.metrics.ValidateAdvanceFrom(r.expected.metrics); err != nil {
		return err
	}
	if r.state.updatedAt.Before(r.expected.updatedAt) {
		return errors.New("run: replacement precedes last update")
	}
	if r.expected.state.IsTerminal() {
		settled := r.expected
		settled.messageMark = r.state.messageMark
		if !settled.Equal(r.state) {
			return errors.New("run: replacement changes settled Run facts")
		}
	}
	return nil
}

// Expected returns the complete aggregate the replacement was derived from.
func (r Replacement) Expected() Run { return r.expected }

// State returns the complete already-decided replacement aggregate.
func (r Replacement) State() Run { return r.state }
