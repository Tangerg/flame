package transcript

import (
	"fmt"
	"time"
)

// Replacement binds a decided Item state to the exact aggregate it was
// derived from. It is built only by applying a transition to that Item, so its
// legality is settled at construction; persistence rejects a different current
// Item rather than overwriting a racing transcript transition.
type Replacement struct {
	expected Item
	state    Item
}

// Replace derives one Replacement by applying transition to expected.
func Replace(expected Item, transition func(Item) (Item, error)) (Replacement, error) {
	if expected.ID() == "" {
		return Replacement{}, fmt.Errorf("%w: replacement carries no Item", ErrIdentityConflict)
	}
	state, err := transition(expected)
	if err != nil {
		return Replacement{}, err
	}
	replacement := Replacement{expected: expected, state: state}
	if err := replacement.validate(); err != nil {
		return Replacement{}, err
	}
	return replacement, nil
}

// validate accepts only states the Item's own transitions can produce. Restore
// can reconstruct historical facts but cannot authorize rewriting a live Item.
func (r Replacement) validate() error {
	if r.expected.ID() == "" {
		return fmt.Errorf("%w: replacement carries no Item", ErrIdentityConflict)
	}
	if r.expected.ID() != r.state.ID() ||
		r.expected.SessionID() != r.state.SessionID() ||
		r.expected.RunID() != r.state.RunID() ||
		!r.expected.OccurredAt().Equal(r.state.OccurredAt()) ||
		r.expected.kind != r.state.kind {
		return fmt.Errorf(
			"%w: replacement changes Item %q ownership",
			ErrIdentityConflict,
			r.expected.ID(),
		)
	}
	if r.expected.Equal(r.state) {
		return nil
	}
	decided := r.expected
	var err error
	switch {
	case decided.kind == QuestionItem && r.state.question != nil:
		decided, err = decided.AnswerQuestion(r.state.question.Answers)
	case decided.kind == ToolCall && decided.status == ItemRunning && r.state.tool != nil:
		if decided.approvalDecision != r.state.approvalDecision {
			decided, err = decided.ResolveToolApproval(r.state.approvalDecision)
			if err != nil {
				break
			}
		}
		if r.state.status != ItemRunning {
			var startedAt time.Time
			if r.state.executionDuration != nil {
				startedAt = r.state.finishedAt.Add(-*r.state.executionDuration)
			}
			decided, err = decided.settleToolCall(
				*r.state.tool, r.state.failure, r.state.status, startedAt, r.state.finishedAt,
			)
		}
	}
	if err != nil {
		return fmt.Errorf("%w: item %q replacement: %w", ErrIdentityConflict, r.expected.ID(), err)
	}
	if !decided.Equal(r.state) {
		return fmt.Errorf("%w: replacement rewrites item %q facts", ErrIdentityConflict, r.expected.ID())
	}
	return nil
}

// Expected returns the complete Item the replacement was derived from.
func (r Replacement) Expected() Item { return r.expected }

// State returns the complete already-decided replacement Item.
func (r Replacement) State() Item { return r.state }
