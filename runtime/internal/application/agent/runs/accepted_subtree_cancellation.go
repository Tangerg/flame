package runs

import (
	"errors"
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
)

// ErrSubtreeCancellationReleased leaves later abandoned work with ordinary Run
// recovery. The original Scope cancellation and subsequent release are proven.
var ErrSubtreeCancellationReleased = errors.New("runs: accepted cancellation has left its Scope release boundary")

// AcceptedSubtreeCancellation retains the consumed tree binding until Scope
// has joined the canceled subtree and released any newly opened Segments.
type AcceptedSubtreeCancellation struct {
	SessionID   string
	Pending     Pending
	TargetRunID string
	Segments    []AcceptedSegment
}

func (a AcceptedSubtreeCancellation) RootRunID() string { return a.Pending.RootRunID }

// Disposition is the tree's state after the cancellation: removing the last
// external boundary opened continuation Segments, otherwise the tree waits.
func (a AcceptedSubtreeCancellation) Disposition() WaitingSubtreeDisposition {
	if len(a.Segments) == 0 {
		return WaitingSubtreeStaysWaiting
	}
	return WaitingSubtreeResumesRunning
}

func (a AcceptedSubtreeCancellation) targetMemberID() string {
	for _, member := range a.Pending.Continuations {
		if member.RunID == a.TargetRunID {
			return member.MemberID
		}
	}
	return ""
}

func (a AcceptedSubtreeCancellation) Validate() error {
	if err := identity.ValidateSession(a.SessionID); err != nil {
		return fmt.Errorf("runs: accepted subtree cancellation Session: %w", err)
	}
	if err := a.Pending.Validate(); err != nil {
		return fmt.Errorf("runs: accepted subtree cancellation source: %w", err)
	}
	members := make(map[string]struct{}, len(a.Pending.Continuations))
	for _, continuation := range a.Pending.Continuations {
		members[continuation.RunID] = struct{}{}
	}
	if _, found := members[a.TargetRunID]; !found || a.TargetRunID == a.RootRunID() {
		return errors.New("runs: accepted subtree cancellation target is not an original child")
	}
	segments := make(map[string]struct{}, len(a.Segments))
	delete(members, a.TargetRunID)
	for _, segment := range a.Segments {
		if _, found := members[segment.RunID]; !found {
			return errors.New("runs: accepted subtree cancellation has a duplicate or foreign Segment Run")
		}
		if err := identity.ValidateSegment(segment.SegmentID); err != nil {
			return fmt.Errorf("runs: accepted subtree cancellation Segment: %w", err)
		}
		if _, duplicate := segments[segment.SegmentID]; duplicate {
			return errors.New("runs: accepted subtree cancellation repeats a Segment")
		}
		segments[segment.SegmentID] = struct{}{}
		delete(members, segment.RunID)
	}
	if len(a.Segments) != 0 && a.Segments[len(a.Segments)-1].RunID != a.RootRunID() {
		return errors.New("runs: accepted subtree cancellation opening must end with the root Segment")
	}
	return nil
}

func (w WaitingSubtreeCancellationCommit) AcceptedCancellation() AcceptedSubtreeCancellation {
	accepted := AcceptedSubtreeCancellation{
		SessionID: w.state.SessionID, Pending: w.ExpectedPending(), TargetRunID: w.state.TargetRunID,
	}
	if w.state.Resume != nil {
		for _, replacement := range w.state.Resume.Runs {
			value := replacement.State()
			accepted.Segments = append(accepted.Segments, AcceptedSegment{RunID: value.ID(), SegmentID: value.ActiveSegmentID()})
		}
	}
	return accepted
}
