package terminal

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/oolong/core/program"

	"github.com/Tangerg/flame/cli/internal/application/agent/mutation"
	runworkflow "github.com/Tangerg/flame/cli/internal/application/agent/run"
	"github.com/Tangerg/flame/cli/internal/application/retry"
	"github.com/Tangerg/flame/cli/internal/domain/agent"
)

// followRecoveredSession closes the read-then-subscribe gap when the terminal
// opens an already-running session. The background owner attaches first, takes
// a second authoritative read, atomically installs it on the UI thread, and
// only then starts consuming the attached tail.
func (a *app) followRecoveredSession() {
	dispatcher := a.loop.Dispatcher()
	a.startFollowing(func(ctx context.Context, lease operationLease) {
		follower := streamFollower{
			app: a, ctx: ctx, dispatcher: dispatcher, lease: lease,
			applyEvent: a.apply, policy: a.reconnectPolicy,
		}
		recovered, ok := follower.restoreAttachedSession(a.session.current.ID)
		if !ok {
			return
		}
		active := true
		var reconcileErr error
		postErr := post(ctx, dispatcher, func() {
			if !follower.current() {
				active = false
				return
			}
			reconcileErr = a.reconcileRunSnapshot(recovered.Snapshot, recovered.Stream)
		})
		if postErr != nil || reconcileErr != nil {
			follower.postFailure("", errors.Join(postErr, reconcileErr))
			return
		}
		if !active || recovered.Run.Status != agent.RunStatusRunning {
			return
		}
		follower.checkpoint = recovered.Stream.HeadEventID
		follower.runStream(recovered.Stream)
	})
}

type streamFollower struct {
	app        *app
	ctx        context.Context
	dispatcher program.Dispatcher
	lease      operationLease
	open       func(context.Context) (agent.SegmentStream, error)
	applyEvent func(agent.RunEvent) error
	policy     retry.ReconnectPolicy
	opening    streamOpeningObserver
	failures   int
	checkpoint string
}

func (s *streamFollower) current() bool {
	return s != nil && s.app != nil && s.app.operations.Current(s.lease)
}

func (s *streamFollower) restoreAttachedSession(sessionID string) (runworkflow.Recovery, bool) {
	for {
		recovered, err := runworkflow.AttachSession(s.ctx, s.app.runtime, sessionID)
		if err == nil {
			s.failures = 0
			return recovered, true
		}
		if !s.waitBeforeRetry("", fmt.Errorf("restore active session: %w", err)) {
			return runworkflow.Recovery{}, false
		}
	}
}

// eventApplicationError marks a runtime event that reached the terminal but could
// not be folded into its conversation projection. Reconnecting cannot repair an
// invalid or conflicting event; only transport failures are eligible for replay.
type eventApplicationError struct{ err error }

func (e *eventApplicationError) Error() string { return e.err.Error() }
func (e *eventApplicationError) Unwrap() error { return e.err }

func (s *streamFollower) run() {
	var current agent.SegmentStream
	for {
		opened, err := s.open(s.ctx)
		if err == nil {
			current = opened
			s.failures = 0
			break
		}
		if !s.waitBeforeOpenRetry(err) {
			return
		}
	}
	if !s.postOpenAccepted(current) {
		return
	}
	s.runStream(current)
}

func (s *streamFollower) postOpenAccepted(opened agent.SegmentStream) bool {
	if s.opening.accepted == nil {
		return true
	}
	active := true
	err := post(s.ctx, s.dispatcher, func() {
		if !s.current() {
			active = false
			return
		}
		active = s.opening.accepted(opened) == followOpenedStream && s.current()
	})
	return err == nil && active
}

func (s *streamFollower) waitBeforeOpenRetry(cause error) bool {
	s.failures++
	if s.opening.persistent && mutation.AcknowledgementUncertain(cause) {
		return s.postRetryStatus(true) && runtimeRecoveryBackoff.Wait(s.ctx, s.failures) == nil
	}
	delay, shouldRetry, policyErr := s.policy.Next(s.failures, cause)
	if policyErr != nil {
		s.postOpenFailure(policyErr)
		return false
	}
	if !shouldRetry {
		s.postOpenFailure(cause)
		return false
	}
	return s.postRetryStatus(false) && retry.Wait(s.ctx, delay) == nil
}

func (s *streamFollower) runStream(current agent.SegmentStream) {
	if err := current.Validate(); err != nil {
		s.postFailure(current.RunID, err)
		return
	}
	for {
		active, applied, streamErr := s.consume(current.Events)
		if !active {
			return
		}
		if applicationErr, ok := errors.AsType[*eventApplicationError](streamErr); ok {
			s.postFailure(current.RunID, applicationErr.err)
			return
		}
		snapshot, err := s.snapshot()
		if err != nil || !snapshot.active {
			return
		}
		s.checkpoint = snapshot.checkpoint
		if snapshot.phase != agent.ConversationRunning {
			s.finish()
			return
		}
		if streamErr == nil {
			streamErr = fmt.Errorf("%w: segment stream ended without a terminal event", agent.ErrDisconnected)
		}
		if context.Cause(s.ctx) != nil {
			return
		}
		if applied > 0 {
			s.failures = 0
		}
		rebound, ok := s.reconnect(current.RunID, current.SegmentID, streamErr)
		if !ok {
			return
		}
		current = rebound
	}
}

func (s *streamFollower) consume(stream agent.EventStream) (bool, int, error) {
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

func (s *streamFollower) apply(event agent.RunEvent) (bool, error) {
	active := true
	var applyErr error
	err := post(s.ctx, s.dispatcher, func() {
		if !s.current() {
			active = false
			return
		}
		applyErr = s.applyEvent(event)
		s.checkpoint = s.app.execution.conversation.Checkpoint()
	})
	if applyErr != nil {
		applyErr = &eventApplicationError{err: applyErr}
	}
	return active, errors.Join(err, applyErr)
}

type followSnapshot struct {
	active     bool
	checkpoint string
	phase      agent.ConversationPhase
}

type recoveryDisposition uint8

const (
	recoveryStopped recoveryDisposition = iota
	recoveryRetry
	recoveryAttached
)

type recoveryAttempt struct {
	disposition recoveryDisposition
	stream      agent.SegmentStream
	cause       error
}

func (s *streamFollower) snapshot() (followSnapshot, error) {
	snapshot := followSnapshot{active: true}
	err := post(s.ctx, s.dispatcher, func() {
		if !s.current() {
			snapshot.active = false
			return
		}
		snapshot.checkpoint = s.app.execution.conversation.Checkpoint()
		snapshot.phase = s.app.execution.conversation.Phase()
	})
	return snapshot, err
}

func (s *streamFollower) reconnect(runID, segmentID string, cause error) (agent.SegmentStream, bool) {
	for {
		if !s.waitBeforeRetry(runID, cause) {
			return agent.SegmentStream{}, false
		}
		rebound, err := s.app.runtime.SubscribeRun(s.ctx, agent.SubscribeRun{
			RunID: runID, SegmentID: segmentID, AfterEventID: s.checkpoint,
		})
		if err == nil {
			return s.acceptRebound(runID, segmentID, rebound)
		}
		if !runworkflow.RecoveryRequired(err) {
			cause = err
			continue
		}
		recovery := s.recover(runID, cause)
		switch recovery.disposition {
		case recoveryRetry:
			cause = recovery.cause
		case recoveryAttached:
			return recovery.stream, true
		default:
			return agent.SegmentStream{}, false
		}
	}
}

func (s *streamFollower) waitBeforeRetry(runID string, cause error) bool {
	s.failures++
	delay, shouldRetry, policyErr := s.policy.Next(s.failures, cause)
	if policyErr != nil {
		s.postFailure(runID, policyErr)
		return false
	}
	if !shouldRetry {
		s.postFailure(runID, cause)
		return false
	}
	return s.postRetryStatus(false) && retry.Wait(s.ctx, delay) == nil
}

func (s *streamFollower) postRetryStatus(persistent bool) bool {
	err := post(s.ctx, s.dispatcher, func() {
		if s.current() {
			label := fmt.Sprintf("reconnecting %d/%d", s.failures, s.policy.AttemptLimit())
			if persistent {
				label = fmt.Sprintf("confirming delivery · attempt %d", s.failures)
			}
			s.app.status.note(label)
			s.app.syncAnimation()
		}
	})
	return err == nil
}

func (s *streamFollower) acceptRebound(runID, segmentID string, rebound agent.SegmentStream) (agent.SegmentStream, bool) {
	if err := rebound.ValidateSubscription(); err != nil {
		s.postFailure(runID, err)
		return agent.SegmentStream{}, false
	}
	if rebound.RunID != runID || rebound.SegmentID != segmentID {
		s.postFailure(runID, errors.New("runtime rebound a different run segment"))
		return agent.SegmentStream{}, false
	}
	return rebound, true
}

func (s *streamFollower) recover(runID string, cause error) recoveryAttempt {
	recovered, err := runworkflow.RecoverSegment(s.ctx, s.app.runtime, s.app.session.current.ID, runID)
	if err != nil {
		if runworkflow.RecoveryRequired(err) {
			return recoveryAttempt{disposition: recoveryRetry, cause: cause}
		}
		return recoveryAttempt{disposition: recoveryRetry, cause: err}
	}
	active := true
	var reconcileErr error
	postErr := post(s.ctx, s.dispatcher, func() {
		if !s.current() {
			active = false
			return
		}
		reconcileErr = s.app.reconcileRunSnapshot(recovered.Snapshot, recovered.Stream)
	})
	if postErr != nil || reconcileErr != nil {
		s.postFailure(runID, errors.Join(postErr, reconcileErr))
		return recoveryAttempt{disposition: recoveryStopped}
	}
	if !active || recovered.Run.Status != agent.RunStatusRunning {
		return recoveryAttempt{disposition: recoveryStopped}
	}
	s.checkpoint = recovered.Stream.HeadEventID
	return recoveryAttempt{disposition: recoveryAttached, stream: recovered.Stream}
}

func (s *streamFollower) finish() {
	_ = post(s.ctx, s.dispatcher, func() {
		if s.current() {
			s.app.finishFollowing()
		}
	})
}

func (s *streamFollower) postFailure(runID string, err error) {
	if errors.Is(err, context.Canceled) || s.ctx.Err() != nil {
		return
	}
	_ = post(s.ctx, s.dispatcher, func() {
		if !s.current() {
			return
		}
		s.app.fail(err)
		if runID != "" {
			s.app.cancelRuntimePreservingFailure(agent.CancelRun{RunID: runID, Reason: "terminal stream failed"})
		}
	})
}

func (s *streamFollower) postOpenFailure(err error) {
	if errors.Is(err, context.Canceled) || s.ctx.Err() != nil {
		return
	}
	_ = post(s.ctx, s.dispatcher, func() {
		if !s.current() {
			return
		}
		if s.opening.rejected != nil {
			err = s.opening.rejected(err)
			if err == nil {
				return
			}
		}
		s.app.fail(err)
	})
}
