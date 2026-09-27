package runtimefixture

import (
	"context"
	"fmt"
	"slices"

	"github.com/Tangerg/flame/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/runtime/protocol"
)

func (r *Runtime) SubscribeRun(ctx context.Context, in conversation.SubscribeRun) (conversation.SegmentStream, error) {
	if err := in.Validate(); err != nil {
		return conversation.SegmentStream{}, fmt.Errorf("mock: %w", err)
	}
	if err := context.Cause(ctx); err != nil {
		return conversation.SegmentStream{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	run := r.runs[in.RunID]
	if run == nil {
		return conversation.SegmentStream{}, fmt.Errorf("%w: %s", conversation.ErrRunNotFound, in.RunID)
	}
	if run.active != in.SegmentID || run.status != protocol.RunStatusRunning {
		return conversation.SegmentStream{}, fmt.Errorf("%w: run %s is not executing segment %s", conversation.ErrStaleSegment, in.RunID, in.SegmentID)
	}
	segment := run.segments[in.SegmentID]
	if segment == nil {
		return conversation.SegmentStream{}, fmt.Errorf("%w: %s", conversation.ErrStaleSegment, in.SegmentID)
	}

	head := len(segment.events)
	start := head // Empty checkpoint attaches at the current head.
	if in.AfterEventID != "" {
		at := replayIndex(segment.events, in.AfterEventID)
		if at < 0 {
			return conversation.SegmentStream{}, fmt.Errorf("%w: event %s", conversation.ErrReplayUnavailable, in.AfterEventID)
		}
		start = at + 1
	}
	fault, err := r.takeFaultLocked()
	if err != nil {
		return conversation.SegmentStream{}, err
	}
	stream := r.bindSegmentLocked(ctx, run, segment, start, head, "", fault)
	if in.Snapshot {
		snapshot, err := r.sessionSnapshotLocked(in.SessionID)
		if err != nil {
			return conversation.SegmentStream{}, err
		}
		stream.Snapshot = &snapshot
	}
	return stream, nil
}

func replayIndex(events []conversation.RunEvent, eventID string) int {
	for i, event := range events {
		if event.EventID == eventID && conversation.ReplayableEvent(event.Event) {
			return i
		}
	}
	return -1
}

func (r *Runtime) openSegmentLocked(run *runState) *segmentState {
	segment := &segmentState{id: r.identities.next(segmentIdentity), changed: make(chan struct{})}
	run.active = segment.id
	run.segments[segment.id] = segment
	return segment
}

func (r *Runtime) bindSegmentLocked(
	ctx context.Context,
	run *runState,
	segment *segmentState,
	start int,
	replayUntil int,
	userItemID string,
	fault SubscriptionFault,
) conversation.SegmentStream {
	headEventID := ""
	for _, event := range slices.Backward(segment.events) {
		if conversation.ReplayableEvent(event.Event) {
			headEventID = event.EventID
			break
		}
	}
	subscription := &segmentSubscription{
		runtime: r, ctx: ctx, run: run, segment: segment,
		next: start, replayUntil: replayUntil, fault: fault,
	}
	return conversation.SegmentStream{
		RunID: run.id, SegmentID: segment.id, UserItemID: userItemID,
		HeadEventID: headEventID, Events: subscription.stream,
	}
}

type segmentSubscription struct {
	runtime           *Runtime
	ctx               context.Context
	run               *runState
	segment           *segmentState
	next              int
	replayUntil       int
	fault             SubscriptionFault
	position          int
	terminalDelivered bool
}

func (s *segmentSubscription) stream(yield func(conversation.RunEvent, error) bool) {
	for {
		next, closed, changed, terminalErr := s.nextEvent()
		if next != nil {
			if !s.deliver(*next, yield) {
				return
			}
			continue
		}
		if terminalErr != nil {
			yield(conversation.RunEvent{}, terminalErr)
			return
		}
		if closed || !s.awaitChange(changed, yield) {
			return
		}
	}
}

func (s *segmentSubscription) nextEvent() (*conversation.RunEvent, bool, <-chan struct{}, error) {
	s.runtime.mu.Lock()
	defer s.runtime.mu.Unlock()
	for s.next < len(s.segment.events) {
		at := s.next
		s.next++
		event := s.segment.events[at]
		if at < s.replayUntil && !conversation.ReplayableEvent(event.Event) {
			continue
		}
		cloned := event.Clone()
		return &cloned, s.segment.closed, s.segment.changed, nil
	}
	if !s.terminalDelivered && s.segment.terminalErr != nil {
		s.terminalDelivered = true
		return nil, true, s.segment.changed, s.segment.terminalErr
	}
	return nil, s.segment.closed, s.segment.changed, nil
}

func (s *segmentSubscription) deliver(next conversation.RunEvent, yield func(conversation.RunEvent, error) bool) bool {
	s.position++
	if !yield(next, nil) {
		return false
	}
	if s.position == s.fault.After {
		switch s.fault.Kind {
		case FaultDuplicate:
			if !yield(next, nil) {
				return false
			}
		case FaultConflict:
			conflict := next.Clone()
			conflict.Event = conversation.BlockCompleted{Block: conversation.Block{ID: "conflict", RunID: next.RunID, Status: conversation.BlockStatusCompleted, Kind: conversation.BlockNotice, Text: "conflicting replay"}}
			yield(conflict, nil)
			return false
		case FaultDisconnect:
			yield(conversation.RunEvent{}, fmt.Errorf("%w after event %s", conversation.ErrDisconnected, next.EventID))
			return false
		}
	}
	return true
}

func (s *segmentSubscription) awaitChange(changed <-chan struct{}, yield func(conversation.RunEvent, error) bool) bool {
	select {
	case <-changed:
		return true
	case <-s.ctx.Done():
		yield(conversation.RunEvent{}, context.Cause(s.ctx))
		return false
	}
}

func (r *Runtime) takeFaultLocked() (SubscriptionFault, error) {
	if r.fault >= len(r.Faults) {
		return SubscriptionFault{}, nil
	}
	fault := r.Faults[r.fault]
	if fault.After < 1 {
		fault.After = 1
	}
	switch fault.Kind {
	case FaultDisconnect, FaultDuplicate, FaultConflict:
		r.fault++
		return fault, nil
	default:
		return SubscriptionFault{}, fmt.Errorf("mock: unknown subscription fault %q", fault.Kind)
	}
}
