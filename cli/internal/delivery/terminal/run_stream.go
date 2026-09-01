package terminal

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Tangerg/oolong/core/program"
	"github.com/Tangerg/oolong/core/term"

	"github.com/Tangerg/flame/cli/internal/application/agent/mutation"
	runworkflow "github.com/Tangerg/flame/cli/internal/application/agent/run"
	"github.com/Tangerg/flame/cli/internal/application/agent/workbench"
	"github.com/Tangerg/flame/cli/internal/application/retry"
	"github.com/Tangerg/flame/cli/internal/domain/agent"
	"github.com/Tangerg/flame/cli/internal/domain/commandreplay"
	runtimeprotocol "github.com/Tangerg/flame/runtime/protocol"
)

const (
	animationInterval = 100 * time.Millisecond
)

var errStreamFollowerUnavailable = errors.New("stream follower ownership is unavailable")

type activeDurationClock struct {
	carried          time.Duration
	segmentStartedAt time.Time
}

// startRunCallError identifies an error returned by RunLifecycle.StartRun
// itself. Protocol validation failures after a successful call are deliberately
// excluded: the runtime already acknowledged the command, so replaying the
// mutation cannot repair its malformed receipt.
type startRunCallError struct{ err error }

func (s *startRunCallError) Error() string { return s.err.Error() }
func (s *startRunCallError) Unwrap() error { return s.err }

// resumeRunCallError has the same acknowledgement semantics as
// startRunCallError, but its recovery owner is the still-open HITL review.
type resumeRunCallError struct{ err error }

func (r *resumeRunCallError) Error() string { return r.err.Error() }
func (r *resumeRunCallError) Unwrap() error { return r.err }

func (a *activeDurationClock) start(carried time.Duration, at time.Time) {
	a.carried = carried
	a.segmentStartedAt = at
}

func (a *activeDurationClock) elapsed(at time.Time) time.Duration {
	if a.segmentStartedAt.IsZero() {
		return a.carried
	}
	current := at.Sub(a.segmentStartedAt)
	if current < 0 {
		return a.carried
	}
	return a.carried + current
}

func (a *app) startRun(commandID agent.CommandID, message agent.Message, options agent.RunOptions, status string) bool {
	input := agent.StartRun{CommandID: commandID, SessionID: a.session.current.ID, Message: message.Clone(), Options: options.Clone()}
	replay, ready := a.prepareRunStart(input)
	if !ready {
		return false
	}
	a.presentRunStart(status)
	a.followOpening(func(ctx context.Context) (agent.SegmentStream, error) {
		opened, err := openStartRun(
			ctx, a.runtime, input, retry.DisabledReconnectPolicy(), commandReplayAdmission(replay, a.runtimeProfile),
		)
		if err != nil {
			if _, accepted := agent.AcceptedMutationReceipt(err); accepted {
				return agent.SegmentStream{}, err
			}
			return agent.SegmentStream{}, &startRunCallError{err: err}
		}
		if err := opened.ValidateStart(); err != nil {
			return agent.SegmentStream{}, agent.NewAcceptedMutationError(opened, fmt.Errorf("start run: %w", err))
		}
		return opened, nil
	}, streamOpeningObserver{
		persistent: true,
		accepted: func(opened agent.SegmentStream) streamOpeningDisposition {
			a.acceptStartedRun(input, opened)
			return followOpenedStream
		},
		rejected: func(err error) error {
			if receipt, accepted := agent.AcceptedMutationReceipt(err); accepted {
				if identityErr := runtimeprotocol.ValidateRunID(receipt.RunID); identityErr != nil {
					return errors.Join(err, identityErr, a.requeueDefinitivelyRefusedStart(input, err))
				}
				a.execution.openingRunID = receipt.RunID
				a.cancelRuntimePreservingFailure(agent.CancelRun{
					RunID: receipt.RunID, Reason: "runtime returned an invalid start receipt",
				})
			}
			return errors.Join(err, a.requeueDefinitivelyRefusedStart(input, err))
		},
	})
	return true
}

func (a *app) prepareRunStart(input agent.StartRun) (commandreplay.Guard, bool) {
	if err := a.execution.conversation.Starting(); err != nil {
		a.fail(err)
		return commandreplay.Guard{}, false
	}
	replay := commandReplayGuard(a.runtimeProfile)
	if a.workbench == nil {
		return replay, true
	}
	if err := a.workbench.MarkPendingRunDispatching(input.SessionID, input.CommandID, replay); err != nil {
		rollbackErr := a.execution.conversation.CancelStarting()
		a.message("run start blocked: save dispatching run: " + err.Error())
		if rollbackErr != nil {
			a.fail(errors.Join(err, rollbackErr))
		}
		return commandreplay.Guard{}, false
	}
	pending, ok := pendingRunByCommandID(a.workbench.PendingRuns(input.SessionID), input.CommandID)
	if !ok {
		a.fail(errors.New("dispatching run disappeared from the durable outbox"))
		return commandreplay.Guard{}, false
	}
	return pending.Replay, true
}

func (a *app) presentRunStart(status string) {
	a.transcript.Follow()
	a.activity.Reset()
	a.header.SetUsage(agent.Usage{})
	a.prompt.SetBusy(true)
	a.status.active(status)
	a.execution.clock.start(0, time.Now())
	a.syncAnimation()
}

func (a *app) acceptStartedRun(input agent.StartRun, opened agent.SegmentStream) {
	a.execution.openingRunID = opened.RunID
	if a.workbench == nil {
		return
	}
	pending := a.workbench.PendingRuns(input.SessionID)
	if len(pending) == 0 || pending[0].Command.CommandID != input.CommandID {
		return
	}
	if pending[0].State != workbench.PendingRunCanceling {
		return
	}
	a.status.active("canceling")
	a.requestRuntimeCancellation(agent.CancelRun{
		CommandID: pending[0].CancelCommandID,
		RunID:     opened.RunID,
		Reason:    "canceled while start delivery was unconfirmed",
	}, applyRuntimeSettlement)
}

func (a *app) requeueDefinitivelyRefusedStart(input agent.StartRun, failure error) error {
	callFailure, refused := errors.AsType[*startRunCallError](failure)
	_, dispatchingPresent := a.queue.Dispatching(input.SessionID)
	if !refused || mutation.OutcomeUnknown(callFailure.err) || !dispatchingPresent {
		return nil
	}
	var replacement agent.CommandID
	var err error
	if a.workbench != nil {
		replacement, err = a.workbench.RequeuePendingRun(input.SessionID, input.CommandID)
		if err != nil {
			return fmt.Errorf("requeue refused run: %w", err)
		}
	} else {
		replacement, err = agent.NewCommandID()
		if err != nil {
			return fmt.Errorf("prepare refused run for retry: %w", err)
		}
	}
	if err := a.queue.RequeueDispatch(input.SessionID, input.CommandID, replacement); err != nil {
		return fmt.Errorf("reidentify refused run: %w", err)
	}
	return nil
}

func openStartRun(
	ctx context.Context,
	runtime runworkflow.RunLifecycle,
	command agent.StartRun,
	policy retry.ReconnectPolicy,
	admit mutation.Admission,
) (agent.SegmentStream, error) {
	if err := policy.Validate(); err != nil {
		return agent.SegmentStream{}, err
	}
	for attempt := 1; ; attempt++ {
		if admit != nil {
			if err := admit(); err != nil {
				return agent.SegmentStream{}, err
			}
		}
		opened, err := runtime.StartRun(ctx, command)
		if err == nil {
			return opened, nil
		}
		delay, shouldRetry, policyErr := policy.Next(attempt, err)
		if policyErr != nil {
			return agent.SegmentStream{}, policyErr
		}
		if !shouldRetry {
			return agent.SegmentStream{}, err
		}
		if err := retry.Wait(ctx, delay); err != nil {
			return agent.SegmentStream{}, err
		}
	}
}

type streamOpeningDisposition uint8

const (
	rejectOpenedStream streamOpeningDisposition = iota
	followOpenedStream
)

type streamOpeningObserver struct {
	// accepted owns the linearization boundary between command acknowledgement
	// and stream consumption. It may reject a valid runtime stream when the local
	// projection cannot safely install the acknowledged state.
	accepted func(agent.SegmentStream) streamOpeningDisposition
	rejected func(error) error
	// persistent makes retryable opening failures wait for either an
	// acknowledgement or owner cancellation. It is reserved for idempotent
	// mutations whose delivery outcome is ambiguous after a disconnect.
	persistent bool
}

func (a *app) followOpening(
	open func(context.Context) (agent.SegmentStream, error),
	observer streamOpeningObserver,
) {
	a.startFollowing(func(ctx context.Context, lease operationLease) {
		follower := streamFollower{
			app: a, ctx: ctx, dispatcher: a.loop.Dispatcher(), lease: lease,
			open: open, applyEvent: a.apply,
			policy: a.reconnectPolicy,
		}
		follower.opening = observer
		follower.run()
	})
}

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

func post(ctx context.Context, dispatcher program.Dispatcher, fn func()) error {
	finished := make(chan struct{})
	var claimed atomic.Bool
	dispatcher.Post(func() {
		if claimed.CompareAndSwap(false, true) {
			fn()
		}
		close(finished)
	})
	abort := func(err error) error {
		if claimed.CompareAndSwap(false, true) {
			return err
		}
		<-finished
		return err
	}
	select {
	case <-finished:
		return nil
	case <-ctx.Done():
		return abort(context.Cause(ctx))
	case <-dispatcher.Done():
		return abort(program.ErrStopped)
	}
}

func (a *app) apply(event agent.RunEvent) error {
	result, err := a.execution.conversation.ApplyRunEvent(event)
	if err != nil {
		return fmt.Errorf("apply runtime event %s: %w", event.EventID, err)
	}
	if !result.Applied {
		return nil
	}
	if err := a.transcript.ApplyRunEvent(event, a.registry); err != nil {
		return err
	}
	a.applyPresentationEvent(event)
	a.status.setRunningDescendants(a.execution.conversation.RunningDescendants())
	switch event.Event.(type) {
	case agent.SegmentStarted, agent.RunProgress, agent.RunInterrupted, agent.RunSuspended, agent.RunFinished:
		a.refreshOpenTimeline()
	}
	a.transcript.DiscardExcess()
	a.syncAnimation()
	return nil
}

func (a *app) applyPresentationEvent(envelope agent.RunEvent) {
	switch event := envelope.Event.(type) {
	case agent.SegmentStarted:
		if event.Run.Lineage.IsRoot() {
			if settled := a.settleQueuedDispatch(); settled {
				a.execution.openingRunID = ""
				a.status.active("working")
			} else if a.queuedDispatchCanceling() {
				a.status.active("canceling")
			} else {
				a.status.note("working · retrying local settlement")
			}
			a.execution.clock.start(event.Run.Usage.Duration, time.Now())
		}
	case agent.BlockStarted:
		a.noteBlockStarted(event.Block)
	case agent.BlockCompleted:
		if event.Block.Kind == agent.BlockTool {
			a.status.active("working")
		}
	case agent.PlanChanged:
		a.activity.Set(a.execution.conversation.PlanItems())
	case agent.RunProgress:
		if envelope.RunID == a.execution.conversation.RunID() {
			a.header.SetUsage(a.execution.conversation.Usage())
			a.status.progress(event)
		} else if strings.TrimSpace(event.Activity) != "" {
			a.status.active("subagent · " + event.Activity)
		}
	case agent.RunInterrupted:
		if a.execution.conversation.Phase() == agent.ConversationWaiting {
			a.openInteractions(a.execution.conversation.Interactions())
			a.header.SetUsage(a.execution.conversation.Usage())
			a.status.note("waiting for your answers")
		}
	case agent.RunSuspended:
		if a.execution.conversation.Phase() == agent.ConversationWaiting {
			a.openInteractions(a.execution.conversation.Interactions())
			a.header.SetUsage(a.execution.conversation.Usage())
			a.status.note("waiting for your answers")
		}
	case agent.RunFinished:
		if envelope.RunID == a.execution.conversation.RunID() {
			a.noteRunFinished()
		}
	case agent.BlockDelta, agent.ToolArgumentsDelta, agent.CustomEvent:
	default:
	}
}

func (a *app) noteBlockStarted(block agent.Block) {
	if block.Kind == agent.BlockTool && block.Tool != nil {
		label := strings.TrimSpace(block.Tool.Summary)
		if label == "" {
			label = "using " + toolLabel(*block.Tool)
		}
		a.status.active(label)
	}
}

func (a *app) noteRunFinished() {
	a.status.note("finishing run")
	a.header.SetUsage(a.execution.conversation.Usage())
}

func (a *app) finishFollowing() {
	a.execution.following = false
	a.refreshOpenTimeline()
	if a.session.invalidated {
		a.refreshInvalidatedSession(true)
		return
	}
	if a.execution.conversation.Phase() != agent.ConversationIdle || a.execution.conversation.Outcome().Status == "" {
		return
	}
	a.status.settled(a.execution.conversation.Outcome(), a.execution.conversation.Usage())
	a.prompt.SetBusy(false)
	settled := a.settleQueuedDispatch()
	if settled {
		a.execution.openingRunID = ""
	} else if a.queuedDispatchCanceling() || a.execution.pendingCancel != nil {
		a.status.note("canceling")
	} else {
		a.status.note("run complete · retrying local settlement")
	}
	if settled && a.drainQueue() {
		return
	}
	a.raiseAttention(outcomeAttention(a.execution.conversation.Outcome()))
}

func outcomeNotification(outcome agent.Outcome) string {
	switch outcome.Status {
	case agent.OutcomeCompleted:
		return "flame run completed"
	case agent.OutcomeCanceled:
		return "flame run canceled"
	case agent.OutcomeTimedOut, agent.OutcomeMaxSteps, agent.OutcomeMaxBudget:
		return "flame run stopped: " + string(outcome.Status)
	case agent.OutcomeFailed, agent.OutcomeLost:
		return "flame run failed"
	default:
		return ""
	}
}

func (a *app) fail(err error) {
	if err == nil || errors.Is(err, context.Canceled) {
		return
	}
	a.execution.following = false
	a.dismissInteractionProjection()
	a.execution.conversation.Failed(err)
	a.transcript.settleLive(a.execution.conversation.Outcome())
	a.transcript.Append(presentError(a.transcript.theme, err.Error()))
	a.status.settled(a.execution.conversation.Outcome(), a.execution.conversation.Usage())
	a.header.SetUsage(a.execution.conversation.Usage())
	a.prompt.SetBusy(false)
	a.syncAnimation()
	a.raiseAttention(failureAttention())
}

func (a *app) dropStream() {
	a.operations.Cancel(streamOperation)
	a.execution.following = false
}

func (a *app) startFollowing(work func(context.Context, operationLease)) {
	a.dropStream()
	a.execution.following = true
	if a.operations.Go(streamOperation, false, work) {
		return
	}
	a.execution.following = false
	a.fail(errStreamFollowerUnavailable)
}

func (a *app) syncAnimation() {
	running := a.execution.conversation.Phase() == agent.ConversationRunning
	switch {
	case running && a.execution.stopClock == nil:
		a.execution.stopClock = a.loop.Every(animationInterval, func() {
			a.status.tick(a.execution.clock.elapsed(time.Now()))
		})
	case !running && a.execution.stopClock != nil:
		a.execution.stopClock()
		a.execution.stopClock = nil
	}
	state := term.Progress{}
	if running {
		state.State = term.ProgressIndeterminate
	}
	a.loop.Session().SetProgress(state)
}
