package runs

import (
	"errors"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/internal/domain/run/interrupt"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
)

// AcceptedContinuation is the Run-owned binding that lets boot recovery revive an
// accepted but not-yet-delivered continuation after an executor crash. The
// acceptance transaction writes it from the exact open interrupt set it consumed;
// delivery completion or a successful revival deletes it. It retains the hand-off
// routing only — each answer's resolution stays owned by its settled transcript
// Item and is read back by InterruptItemID, so no resolution is stored twice and a
// later edit through the Item cannot diverge from the binding.
type AcceptedContinuation struct {
	SessionID string
	Pending   Pending
}

// NewAcceptedContinuation binds the consumed open interrupt set to its Session as
// the recovery hand-off. It reads only routing from the Pending; the resolutions
// it will carry at revival are read from the settled Items, not copied here.
func NewAcceptedContinuation(sessionID string, pending Pending) (AcceptedContinuation, error) {
	accepted := AcceptedContinuation{SessionID: sessionID, Pending: pending.Clone()}
	if err := accepted.Validate(); err != nil {
		return AcceptedContinuation{}, err
	}
	return accepted, nil
}

func (a AcceptedContinuation) Validate() error {
	if err := resourceid.ValidateSession(a.SessionID); err != nil {
		return fmt.Errorf("runs: accepted continuation session: %w", err)
	}
	if err := a.Pending.Validate(); err != nil {
		return fmt.Errorf("runs: accepted continuation pending: %w", err)
	}
	return nil
}

// RootRunID is the accepted tree's root, which keys the binding.
func (a AcceptedContinuation) RootRunID() string { return a.Pending.RootRunID }

// Clone returns an ownership-isolated copy.
func (a AcceptedContinuation) Clone() AcceptedContinuation {
	a.Pending = a.Pending.Clone()
	return a
}

// answers rebuilds the exact executor-bound answer set from the retained routing
// and the settled Items the acceptance committed. The resolution of each answer is
// read back from its Item, which owns it; this binding owns only the routing, so a
// re-delivered Signal reproduces the same identity and bytes.
func (a AcceptedContinuation) answers(items map[string]transcript.Item) ([]InterruptAnswer, error) {
	answers := make([]InterruptAnswer, 0, len(a.Pending.Bindings))
	for _, binding := range a.Pending.Bindings {
		item, ok := items[binding.InterruptItemID]
		if !ok {
			return nil, fmt.Errorf("runs: accepted continuation item %q is missing", binding.InterruptItemID)
		}
		resolution, err := acceptedItemResolution(item)
		if err != nil {
			return nil, fmt.Errorf("runs: accepted continuation item %q: %w", binding.InterruptItemID, err)
		}
		answers = append(answers, InterruptAnswer{
			InterruptItemID: binding.InterruptItemID,
			MemberID:        binding.MemberID,
			RequestID:       binding.RequestID,
			Resolution:      resolution,
		})
	}
	return answers, nil
}

// acceptedItemResolution reads the committed resolution a settled interrupt Item
// owns. A Question Item carries its answers; a reviewed ToolCall carries its full
// approval resolution. The derivation is the exact inverse of the acceptance
// write, so a re-delivered Signal reproduces the same identity and bytes.
func acceptedItemResolution(item transcript.Item) (interrupt.Resolution, error) {
	if question, ok := item.Question(); ok {
		if !question.Answered() {
			return interrupt.Resolution{}, errors.New("question item is not answered")
		}
		return interrupt.Resolution{Answers: question.Answers}, nil
	}
	resolution := item.ApprovalResolution()
	if resolution.Decision == "" {
		return interrupt.Resolution{}, errors.New("approval item has no accepted resolution")
	}
	return interrupt.Resolution{
		Approved:      resolution.Decision == approval.Allow,
		Arguments:     resolution.EditedArguments,
		Reason:        resolution.Reason,
		RememberScope: resolution.RememberScope,
	}, nil
}
