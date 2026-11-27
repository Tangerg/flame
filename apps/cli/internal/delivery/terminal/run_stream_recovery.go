package terminal

import (
	"context"
	"errors"
	"fmt"

	runworkflow "github.com/Tangerg/flame/apps/cli/internal/application/agent/run"
	"github.com/Tangerg/flame/apps/cli/internal/application/mutation"
	"github.com/Tangerg/flame/apps/cli/internal/application/retry"
	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

func (r *runs) followRecoveredSession() {
	sessionID := r.session.current.ID
	r.startFollowing(func(ctx context.Context, lease operationLease) {
		follower := streamFollower{
			runs: r, ctx: ctx, lease: lease, sessionID: sessionID,
			applyEvent: r.apply,
		}
		follower.followAttachedSession()
	})
}

func (s *streamFollower) followAttachedSession() {
	recovered, ok := s.restoreAttachedSession(s.sessionID)
	if !ok {
		return
	}
	if !s.adoptRecovery(recovered) {
		return
	}
	s.runStream(recovered.Stream)
}

type streamFollower struct {
	runs       *runs
	ctx        context.Context
	lease      operationLease
	sessionID  string
	open       func(context.Context) (runworkflow.Opening, error)
	applyEvent func(conversation.RunEvent) error

	opening  streamOpeningObserver
	failures int
}

func (s *streamFollower) current() bool {
	return s != nil && s.runs != nil && s.runs.tasks.operations.Current(s.lease)
}

func (s *streamFollower) restoreAttachedSession(sessionID string) (runworkflow.Recovery, bool) {
	for {
		recovered, err := runworkflow.AttachSession(s.ctx, s.runs.runtime, sessionID)
		if err == nil {
			s.failures = 0
			return recovered, true
		}
		if runworkflow.RecoveryRequired(err) {
			s.failures++
			if !s.postRetryStatus(awaitingReconnect) || runtimeRecoveryBackoff.Wait(s.ctx, s.failures) != nil {
				return runworkflow.Recovery{}, false
			}
			continue
		}
		if !s.waitBeforeRetry(fmt.Errorf("restore active session: %w", err)) {
			return runworkflow.Recovery{}, false
		}
	}
}

type eventApplicationError struct{ err error }

func (e *eventApplicationError) Error() string { return e.err.Error() }
func (e *eventApplicationError) Unwrap() error { return e.err }

func (s *streamFollower) run() {
	var opening runworkflow.Opening
	for {
		opened, err := s.open(s.ctx)
		if err == nil {
			opening = opened
			s.failures = 0
			break
		}
		if !s.waitBeforeOpenRetry(err) {
			return
		}
	}
	if s.postOpenAccepted(opening) != followOpenedStream {
		return
	}
	stream, attached := opening.Stream()
	if !attached {
		stream, attached = s.recoverStream(opening.RunID())
	}
	if attached {
		s.runStream(stream)
	}
}

func (s *streamFollower) postOpenAccepted(opened runworkflow.Opening) streamOpeningDisposition {
	if s.opening.accepted == nil {
		return followOpenedStream
	}
	disposition := rejectOpenedStream
	err := s.runs.tasks.deliver(s.ctx, func() {
		if !s.current() {
			return
		}
		disposition = s.opening.accepted(opened)
		if !s.current() {
			disposition = rejectOpenedStream
		}
	})
	if err != nil {
		return rejectOpenedStream
	}
	return disposition
}

func (s *streamFollower) waitBeforeOpenRetry(cause error) bool {
	s.failures++
	if s.opening.persistent && mutation.AcknowledgementUncertain(cause) {
		return s.postRetryStatus(awaitingDelivery) && runtimeRecoveryBackoff.Wait(s.ctx, s.failures) == nil
	}
	delay, shouldRetry, policyErr := retry.ReconnectDelay(s.failures, cause)
	if policyErr != nil {
		s.postOpenFailure(policyErr)
		return false
	}
	if !shouldRetry {
		s.postOpenFailure(cause)
		return false
	}
	return s.postRetryStatus(awaitingReconnect) && retry.Wait(s.ctx, delay) == nil
}

func (s *streamFollower) runStream(current conversation.SegmentStream) {
	if err := current.Validate(); err != nil {
		s.postFailure(err)
		return
	}
	for {
		active, applied, streamErr := s.consume(current.Events)
		if !active {
			return
		}
		if applicationErr, ok := errors.AsType[*eventApplicationError](streamErr); ok {
			s.postFailure(applicationErr.err)
			return
		}
		snapshot, err := s.snapshot()
		if err != nil || !snapshot.active {
			return
		}
		if !runworkflow.RecoveryRequired(streamErr) {
			if snapshot.phase != conversation.Running {
				s.finish()
				return
			}
			if streamErr == nil {
				streamErr = fmt.Errorf("%w: segment stream ended without a terminal event", conversation.ErrDisconnected)
			}
			if context.Cause(s.ctx) != nil {
				return
			}
			if applied > 0 {
				s.failures = 0
			}
		}
		request := conversation.SubscribeRun{
			RunID: current.RunID, SegmentID: current.SegmentID, AfterEventID: snapshot.checkpoint,
		}
		rebound, ok := s.reconnect(request, streamErr)
		if !ok {
			return
		}
		current = rebound
	}
}

func (s *streamFollower) consume(stream conversation.EventStream) (bool, int, error) {
	applied := 0
	for event, err := range stream {
		if err != nil {
			return true, applied, err
		}
		accepted, err := s.apply(event)
		if err != nil || !accepted {
			return accepted, applied, err
		}
		applied++
	}
	return true, applied, nil
}

func (s *streamFollower) apply(event conversation.RunEvent) (bool, error) {
	active := true
	var applyErr error
	err := s.runs.tasks.deliver(s.ctx, func() {
		if !s.current() {
			active = false
			return
		}
		applyErr = s.applyEvent(event)
	})
	if applyErr != nil {
		applyErr = &eventApplicationError{err: applyErr}
	}
	return active, errors.Join(err, applyErr)
}

type followSnapshot struct {
	active     bool
	checkpoint string
	phase      conversation.Phase
}

func (s *streamFollower) snapshot() (followSnapshot, error) {
	snapshot := followSnapshot{active: true}
	err := s.runs.tasks.deliver(s.ctx, func() {
		if !s.current() {
			snapshot.active = false
			return
		}
		snapshot.checkpoint = s.runs.execution.conversation.Checkpoint()
		snapshot.phase = s.runs.execution.conversation.Phase()
	})
	return snapshot, err
}

func (s *streamFollower) reconnect(request conversation.SubscribeRun, cause error) (conversation.SegmentStream, bool) {
	for {
		if runworkflow.RecoveryRequired(cause) {
			return s.recoverStream(request.RunID)
		}
		if !s.waitBeforeRetry(cause) {
			return conversation.SegmentStream{}, false
		}
		rebound, err := s.runs.runtime.SubscribeRun(s.ctx, request)
		if err == nil {
			return s.acceptRebound(request.RunID, request.SegmentID, rebound)
		}
		cause = err
	}
}

func (s *streamFollower) waitBeforeRetry(cause error) bool {
	s.failures++
	delay, shouldRetry, policyErr := retry.ReconnectDelay(s.failures, cause)
	if policyErr != nil {
		s.postFailure(policyErr)
		return false
	}
	if !shouldRetry {
		s.postFailure(cause)
		return false
	}
	return s.postRetryStatus(awaitingReconnect) && retry.Wait(s.ctx, delay) == nil
}

type retryWait uint8

const (
	awaitingReconnect retryWait = iota
	awaitingDelivery
)

func (w retryWait) status(attempt int) string {
	if w == awaitingDelivery {
		return fmt.Sprintf("confirming delivery · attempt %d", attempt)
	}
	return fmt.Sprintf("reconnecting · attempt %d", attempt)
}

func (s *streamFollower) postRetryStatus(wait retryWait) bool {
	err := s.runs.tasks.deliver(s.ctx, func() {
		if s.current() {
			s.runs.status.note(wait.status(s.failures))
			s.runs.syncAnimation()
		}
	})
	return err == nil
}

func (s *streamFollower) acceptRebound(runID, segmentID string, rebound conversation.SegmentStream) (conversation.SegmentStream, bool) {
	if err := rebound.ValidateSubscription(); err != nil {
		s.postFailure(err)
		return conversation.SegmentStream{}, false
	}
	if rebound.RunID != runID || rebound.SegmentID != segmentID {
		s.postFailure(errors.New("runtime rebound a different run segment"))
		return conversation.SegmentStream{}, false
	}
	return rebound, true
}

func (s *streamFollower) recoverStream(runID string) (conversation.SegmentStream, bool) {
	for {
		recovered, err := runworkflow.RecoverSegment(s.ctx, s.runs.runtime, s.sessionID, runID)
		if err == nil {
			return recovered.Stream, s.adoptRecovery(recovered)
		}
		if runworkflow.RecoveryRequired(err) {
			s.failures++
			if !s.postRetryStatus(awaitingReconnect) || runtimeRecoveryBackoff.Wait(s.ctx, s.failures) != nil {
				return conversation.SegmentStream{}, false
			}
		} else if !s.waitBeforeRetry(err) {
			return conversation.SegmentStream{}, false
		}
	}
}

// adoptRecovery reconciles the recovered snapshot on the UI goroutine and
// reports whether its run is still live for this follower to stream.
func (s *streamFollower) adoptRecovery(recovered runworkflow.Recovery) bool {
	active := true
	var reconcileErr error
	postErr := s.runs.tasks.deliver(s.ctx, func() {
		if !s.current() {
			active = false
			return
		}
		reconcileErr = s.runs.sessions.reconcileRunSnapshot(recovered.Snapshot, recovered.Stream)
	})
	if postErr != nil || reconcileErr != nil {
		s.postFailure(errors.Join(postErr, reconcileErr))
		return false
	}
	return active && recovered.Run.Status == protocol.RunStatusRunning
}

func (s *streamFollower) finish() {
	_ = s.runs.tasks.deliver(s.ctx, func() {
		if s.current() {
			s.runs.finishFollowing()
		}
	})
}

func (s *streamFollower) postFailure(err error) {
	if errors.Is(err, context.Canceled) || s.ctx.Err() != nil {
		return
	}
	_ = s.runs.tasks.deliver(s.ctx, func() {
		if !s.current() {
			return
		}
		s.runs.fail(err)
	})
}

func (s *streamFollower) postOpenFailure(err error) {
	if errors.Is(err, context.Canceled) || s.ctx.Err() != nil {
		return
	}
	_ = s.runs.tasks.deliver(s.ctx, func() {
		if !s.current() {
			return
		}
		if s.opening.rejected != nil {
			err = s.opening.rejected(err)
			if err == nil {
				return
			}
		}
		s.runs.fail(err)
	})
}
