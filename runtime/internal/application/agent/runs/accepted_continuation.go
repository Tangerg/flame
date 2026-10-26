package runs

import (
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/flame/packages/go-sdk/resourceid"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/interrupt"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
)

// AcceptedContinuation is the Run-owned binding that lets boot recovery revive an
// accepted but not-yet-delivered continuation after an executor crash. The
// acceptance transaction writes it from the exact open interrupt set it consumed;
// the next committed waiting or terminal boundary deletes it. Resume intent is
// process-local, so keeping the binding until that boundary preserves recovery
// when the latest acknowledged tree is still paused. It retains the hand-off
// routing only — each answer's resolution stays owned by its settled transcript
// Item and is read back by InterruptItemID, so no resolution is stored twice and a
// later edit through the Item cannot diverge from the binding.
type AcceptedContinuation struct {
	SessionID   string
	Pending     Pending
	Segments    []AcceptedSegment
	InputItemID string
}

type AcceptedSegment struct {
	RunID     string
	SegmentID string
}

// NewAcceptedContinuation reads only routing from the Pending; the resolutions it
// carries at revival are read from the settled Items, never copied here.
func NewAcceptedContinuation(sessionID string, pending Pending, resume run.TreeResumeDraft, inputItemID string) (AcceptedContinuation, error) {
	if err := resume.ValidateForTree(sessionID, pending.RootRunID); err != nil {
		return AcceptedContinuation{}, err
	}
	accepted := AcceptedContinuation{SessionID: sessionID, Pending: pending.Clone(), InputItemID: inputItemID}
	for _, replacement := range resume.Runs {
		value := replacement.State()
		accepted.Segments = append(accepted.Segments, AcceptedSegment{RunID: value.ID(), SegmentID: value.ActiveSegmentID()})
	}
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
	if len(a.Segments) != len(a.Pending.Continuations) {
		return errors.New("runs: accepted continuation segment set differs from its members")
	}
	seen := make(map[string]bool, len(a.Segments))
	for index, segment := range a.Segments {
		if segment.RunID != a.Pending.Continuations[index].RunID {
			return errors.New("runs: accepted continuation segment order differs from its members")
		}
		if err := resourceid.ValidateSegment(segment.SegmentID); err != nil {
			return err
		}
		if seen[segment.SegmentID] {
			return errors.New("runs: accepted continuation repeats a segment")
		}
		seen[segment.SegmentID] = true
	}
	if a.InputItemID != "" {
		if err := resourceid.ValidateItem(a.InputItemID); err != nil {
			return err
		}
	}
	return nil
}

func (a AcceptedContinuation) RootRunID() string { return a.Pending.RootRunID }

func (a AcceptedContinuation) Clone() AcceptedContinuation {
	a.Pending = a.Pending.Clone()
	a.Segments = slices.Clone(a.Segments)
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
		member, _ := continuationForMember(a.Pending.Continuations, binding.MemberID)
		if item.SessionID() != a.SessionID || item.RunID() != member.RunID {
			return nil, errors.New("runs: accepted continuation answer belongs to another owner")
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
		return interrupt.NewQuestionAnswer(question.Answers)
	}
	resolution := item.ApprovalResolution()
	if resolution.Decision == "" {
		return interrupt.Resolution{}, errors.New("approval item has no accepted resolution")
	}
	return interrupt.NewApprovalAnswer(resolution)
}
