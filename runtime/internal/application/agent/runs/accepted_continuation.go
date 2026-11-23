package runs

import (
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
)

type AcceptedContinuation struct {
	SessionID   identity.SessionID
	Pending     Pending
	Segments    []AcceptedSegment
	InputItemID identity.ItemID
}

type AcceptedSegment struct {
	RunID     identity.RunID
	SegmentID identity.SegmentID
}

func NewAcceptedContinuation(sessionID identity.SessionID, pending Pending, resume run.TreeResumeDraft, inputItemID identity.ItemID) (AcceptedContinuation, error) {
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
	if err := a.SessionID.Validate(); err != nil {
		return fmt.Errorf("runs: accepted continuation session: %w", err)
	}
	if err := a.Pending.Validate(); err != nil {
		return fmt.Errorf("runs: accepted continuation pending: %w", err)
	}
	if len(a.Segments) != len(a.Pending.Continuations) {
		return errors.New("runs: accepted continuation segment set differs from its members")
	}
	seen := make(map[identity.SegmentID]bool, len(a.Segments))
	for index, segment := range a.Segments {
		if segment.RunID != a.Pending.Continuations[index].RunID {
			return errors.New("runs: accepted continuation segment order differs from its members")
		}
		if err := segment.SegmentID.Validate(); err != nil {
			return err
		}
		if seen[segment.SegmentID] {
			return errors.New("runs: accepted continuation repeats a segment")
		}
		seen[segment.SegmentID] = true
	}
	return nil
}

func (a AcceptedContinuation) RootRunID() identity.RunID { return a.Pending.RootRunID }

func (a AcceptedContinuation) Clone() AcceptedContinuation {
	a.Pending = a.Pending.Clone()
	a.Segments = slices.Clone(a.Segments)
	return a
}
