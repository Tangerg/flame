package terminal

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

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
	a.execution.projectionFailed = false
	a.transcript.Follow()
	a.activity.Reset()
	a.header.SetUsage(agent.Usage{})
	a.prompt.SetBusy(true)
	a.status.beginRun(status)
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
	sessionID := a.session.current.ID
	a.startFollowing(func(ctx context.Context, lease operationLease) {
		follower := streamFollower{
			app: a, ctx: ctx, dispatcher: a.loop.Dispatcher(), lease: lease, sessionID: sessionID,
			open: open, applyEvent: a.apply,
			policy: a.reconnectPolicy,
		}
		follower.opening = observer
		follower.run()
	})
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
			a.observeCurrentRunStatus()
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
			a.observeCurrentRunStatus()
			a.status.progress(event)
		} else if strings.TrimSpace(event.Activity) != "" {
			a.status.active("subagent · " + event.Activity)
		}
	case agent.RunInterrupted:
		if a.execution.conversation.Phase() == agent.ConversationWaiting {
			a.openInteractions(a.execution.conversation.Interactions())
			a.header.SetUsage(a.execution.conversation.Usage())
			a.observeCurrentRunStatus()
			a.status.note("waiting for your answers")
		}
	case agent.RunSuspended:
		if a.execution.conversation.Phase() == agent.ConversationWaiting {
			a.openInteractions(a.execution.conversation.Interactions())
			a.header.SetUsage(a.execution.conversation.Usage())
			a.observeCurrentRunStatus()
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
	a.observeCurrentRunStatus()
	a.status.note("finishing run")
	a.header.SetUsage(a.execution.conversation.Usage())
}

func (a *app) finishFollowing() {
	a.execution.following = false
	a.execution.projectionFailed = false
	a.refreshOpenTimeline()
	if a.session.invalidated {
		a.refreshInvalidatedSession(true)
		return
	}
	if a.execution.conversation.Phase() != agent.ConversationIdle || a.execution.conversation.Outcome().Status == "" {
		return
	}
	a.settleCurrentRunStatus()
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
	if a.execution.conversation.Phase() == agent.ConversationRunning &&
		a.execution.conversation.RunID() == "" && a.execution.openingRunID == "" {
		err = errors.Join(err, a.execution.conversation.CancelStarting())
	}
	a.transcript.rejectLivePresentation()
	a.transcript.Append(presentError(a.transcript.theme, err.Error()))
	a.header.SetUsage(a.execution.conversation.Usage())
	blocked := a.runAdmissionBlocked()
	a.execution.projectionFailed = a.execution.conversation.Busy()
	a.prompt.SetBusy(blocked)
	a.status.fail(err.Error(), blocked)
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
	running := a.execution.conversation.Phase() == agent.ConversationRunning && a.execution.following
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
