package runs

import (
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
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
	if err := identity.ValidateSession(a.SessionID); err != nil {
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
		if err := identity.ValidateSegment(segment.SegmentID); err != nil {
			return err
		}
		if seen[segment.SegmentID] {
			return errors.New("runs: accepted continuation repeats a segment")
		}
		seen[segment.SegmentID] = true
	}
	if a.InputItemID != "" {
		if err := identity.ValidateItem(a.InputItemID); err != nil {
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
