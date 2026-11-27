package terminal

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	runworkflow "github.com/Tangerg/flame/apps/cli/internal/application/agent/run"
	"github.com/Tangerg/flame/apps/cli/internal/application/mutation"
	"github.com/Tangerg/flame/apps/cli/internal/application/workbench"
	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/oolong/ui/core/term"
)

const (
	animationInterval = 100 * time.Millisecond
)

var errStreamFollowerUnavailable = errors.New("stream follower ownership is unavailable")

type activeDurationClock struct {
	carried          time.Duration
	segmentStartedAt time.Time
}

type startRunCallError struct{ err error }

func (s *startRunCallError) Error() string { return s.err.Error() }
func (s *startRunCallError) Unwrap() error { return s.err }

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

func (r *runs) startQueuedRun(pending workbench.PendingRun, status string) bool {
	if pending.Dispatch != nil {
		return r.startPreparedRun(pending, nil, status)
	}
	started := r.tasks.sessionFence(inputPreparationOperation,
		func(ctx context.Context) (*workbench.PreparedInput, error) {
			blocks, err := r.runtime.PrepareInput(ctx, pending.Command.Message)
			if err != nil {
				return nil, err
			}
			return r.workbench.PrepareInput(ctx, pending.Command.Message, blocks)
		},
		func(prepared *workbench.PreparedInput, err error) {
			if err == nil && r.startPreparedRun(pending, prepared, status) {
				return
			}
			r.queue.ReleasePreparation(pending.Command.SessionID)
			r.syncQueue()
			r.prompt.SetBusy(r.runAdmissionBlocked())
			if err != nil {
				r.link.message("run start blocked: prepare input: " + err.Error())
			}
		},
	)
	if started {
		r.prompt.SetBusy(true)
		r.status.note("preparing input")
	}
	return started
}

func (r *runs) startPreparedRun(pending workbench.PendingRun, prepared *workbench.PreparedInput, status string) bool {
	if err := r.execution.conversation.Starting(); err != nil {
		r.fail(err)
		return false
	}
	if pending.Dispatch == nil {
		replay, err := r.replayPolicy.NewGuard()
		if err != nil {
			r.fail(errors.Join(err, r.execution.conversation.StopStarting()))
			return false
		}
		command := pending.Command
		pending, err = r.queue.PrepareDispatch(command.SessionID, command.CommandID, replay, prepared)
		if err != nil {
			r.link.message("run start blocked: save dispatching run: " + err.Error())
			if rollbackErr := r.execution.conversation.StopStarting(); rollbackErr != nil {
				r.fail(errors.Join(err, rollbackErr))
			}
			return false
		}
	}
	input := pending.Command

	r.presentRunStart(status)
	r.followOpening(input.SessionID, func(ctx context.Context) (runworkflow.Opening, error) {
		opened, err := runworkflow.OpenPendingStart(ctx, r.runtime, r.workbench, pending, r.replayPolicy)
		if err != nil {
			return runworkflow.Opening{}, &startRunCallError{err: err}
		}
		return opened, nil
	}, streamOpeningObserver{
		persistent: true,
		accepted: func(opened runworkflow.Opening) streamOpeningDisposition {
			r.acceptStartedRun(input, opened.RunID())
			return followOpenedStream
		},
		rejected: func(err error) error {
			if errors.Is(err, conversation.ErrOpeningRetired) {
				return errors.Join(err, r.retireQueuedCommand(input.SessionID, input.CommandID))
			}
			return errors.Join(err, r.requeueDefinitivelyRefusedStart(input, err))
		},
	})
	return true
}

func (r *runs) presentRunStart(status string) {
	r.execution.trustProjection()
	r.transcript().Follow()
	r.activity.Reset()
	r.header.SetUsage(conversation.Usage{})
	r.prompt.SetBusy(true)
	r.status.beginRun(status)
	r.execution.clock.start(0, time.Now())
	r.syncAnimation()
}

func (r *runs) acceptStartedRun(input authoring.StartRun, runID string) {
	r.execution.opening = &openingReceipt{SessionID: input.SessionID, RunID: runID, CommandID: input.CommandID}
	pending, found := r.queue.Head(input.SessionID)
	if !found || pending.Command.CommandID != input.CommandID {
		return
	}
	cancellation := pending.Cancellation()
	if cancellation == nil {
		r.settleQueuedDispatch(input.SessionID, input.CommandID)
		return
	}
	r.status.active(statusCanceling)
	r.cancelRuntime(conversation.CancelRun{
		CommandID: cancellation.CommandID,
		RunID:     runID,
		Reason:    unconfirmedStartCancellationReason,
	})
}

func (r *runs) requeueDefinitivelyRefusedStart(input authoring.StartRun, failure error) error {
	callFailure, refused := errors.AsType[*startRunCallError](failure)
	_, dispatchingPresent := r.queue.Dispatching(input.SessionID)
	if !refused || mutation.OutcomeUnknown(callFailure.err) || !dispatchingPresent {
		return nil
	}
	if err := r.queue.RequeueDispatch(input.SessionID, input.CommandID); err != nil {
		return fmt.Errorf("requeue refused run: %w", err)
	}
	return nil
}

type streamOpeningDisposition uint8

const (
	rejectOpenedStream streamOpeningDisposition = iota
	followOpenedStream
)

type streamOpeningObserver struct {
	accepted   func(runworkflow.Opening) streamOpeningDisposition
	rejected   func(error) error
	persistent bool
}

func (r *runs) followOpening(
	sessionID string,
	open func(context.Context) (runworkflow.Opening, error),
	observer streamOpeningObserver,
) {
	r.startFollowing(func(ctx context.Context, lease operationLease) {
		follower := streamFollower{
			runs: r, ctx: ctx, lease: lease, sessionID: sessionID,
			open: open, applyEvent: r.apply,
		}
		follower.opening = observer
		follower.run()
	})
}

func (r *runs) apply(event conversation.RunEvent) error {
	result, err := r.execution.conversation.ApplyRunEvent(event)
	if err != nil {
		return fmt.Errorf("apply runtime event %s: %w", event.EventID, err)
	}
	if !result.Applied {
		return nil
	}
	if err := r.transcript().ApplyRunEvent(event); err != nil {
		return err
	}
	r.applyPresentationEvent(event)
	r.steering.observeSteerEvent(event)
	r.status.setRunningDescendants(r.execution.conversation.RunningDescendants())
	switch event.Event.(type) {
	case conversation.SegmentStarted, conversation.RunProgress, conversation.SegmentFinished:
		r.link.refreshOpenTimeline()
	}
	r.transcript().DiscardExcess()
	r.syncAnimation()
	return nil
}

func (r *runs) applyPresentationEvent(envelope conversation.RunEvent) {
	switch event := envelope.Event.(type) {
	case conversation.SegmentStarted:
		if event.Run.Lineage.IsRoot() {
			r.noteRootSegmentStarted(event)
		}
	case conversation.BlockStarted:
		r.noteBlockStarted(event.Block)
	case conversation.BlockCompleted:
		if event.Block.Kind == conversation.BlockTool {
			r.status.active(statusWorking)
		}
	case conversation.PlanChanged:
		r.activity.Set(r.execution.conversation.PlanItems())
	case conversation.RunProgress:
		r.noteProgress(envelope.RunID, event)
	case conversation.SegmentFinished:
		r.noteSegmentFinished(envelope.RunID, event)
	case conversation.BlockDelta, conversation.ToolArgumentsDelta:
	default:
	}
}

func (r *runs) noteRootSegmentStarted(event conversation.SegmentStarted) {
	r.observeCurrentRunStatus()
	settling := r.tasks.operations.Active(pendingRunSettlementOperation)
	if r.execution.opening != nil && r.execution.opening.RunID == event.Run.ID && !settling {
		r.execution.opening = nil
	}
	switch {
	case r.queuedDispatchCanceling():
		r.status.active(statusCanceling)
	case settling:
		r.status.note(statusWorking + " · retrying local settlement")
	default:
		r.status.active(statusWorking)
	}
	r.execution.clock.start(event.Run.Usage.Duration, time.Now())
}

func (r *runs) noteProgress(runID string, event conversation.RunProgress) {
	if runID == r.execution.conversation.RunID() {
		r.header.SetUsage(r.execution.conversation.Usage())
		r.observeCurrentRunStatus()
		r.status.progress(event)
	} else if event.Activity != nil {
		r.status.active("subagent · " + activityLabel(event.Activity))
	}
}

func (r *runs) noteSegmentFinished(runID string, event conversation.SegmentFinished) {
	switch {
	case event.Run.Status == protocol.RunStatusFinished:
		if runID == r.execution.conversation.RunID() {
			r.noteRunFinished()
		}
	case r.execution.conversation.Phase() == conversation.Waiting:
		r.openInterrupts(r.execution.conversation.Interrupts())
		r.header.SetUsage(r.execution.conversation.Usage())
		r.observeCurrentRunStatus()
		r.status.note(statusWaitingForAnswers)
	}
}

func (r *runs) noteBlockStarted(block conversation.Block) {
	if block.Kind == conversation.BlockTool && block.Tool != nil {
		label := strings.TrimSpace(block.Tool.Summary)
		if label == "" {
			label = "using " + toolLabel(*block.Tool)
		}
		r.status.active(label)
	}
}

func (r *runs) noteRunFinished() {
	r.observeCurrentRunStatus()
	r.status.note("finishing run")
	r.header.SetUsage(r.execution.conversation.Usage())
}

func (r *runs) finishFollowing() {
	r.execution.observation = observationDetached
	r.link.refreshOpenTimeline()
	if r.session.freshness == sessionStale {
		r.sessions.refreshInvalidatedSession(refreshSettlingRun)
		return
	}
	if r.execution.conversation.Phase() != conversation.Idle || r.execution.conversation.Outcome().Status == "" {
		return
	}
	r.settleCurrentRunStatus()
	r.prompt.SetBusy(false)
	_, dispatching := r.queue.Dispatching(r.session.current.ID)
	if !dispatching {
		r.execution.opening = nil
	} else if r.queuedDispatchCanceling() || r.execution.pendingCancel != nil {
		r.status.note(statusCanceling)
	} else if r.tasks.operations.Active(pendingRunSettlementOperation) {
		r.status.note("run complete · retrying local settlement")
	} else {
		r.status.note("pending prompt outcome unknown")
	}
	if !dispatching && r.drainQueue() {
		return
	}
	r.steering.refreshSteerPresentation()
	r.link.raiseAttention(outcomeAttention(r.execution.conversation.Outcome()))
}

func outcomeNotification(outcome conversation.Outcome) string {
	switch outcome.Status {
	case protocol.OutcomeCompleted:
		return "flame run completed"
	case protocol.OutcomeCanceled:
		return "flame run canceled"
	case protocol.OutcomeTimedOut:
		return "flame run stopped: " + string(outcome.Status)
	case protocol.OutcomeFailed, protocol.OutcomeLost:
		return "flame run failed"
	default:
		return ""
	}
}

func (r *runs) fail(err error) {
	if err == nil || errors.Is(err, context.Canceled) {
		return
	}
	r.execution.stopFollowing()
	r.prompts.dismiss()
	if r.execution.conversation.Phase() == conversation.Running &&
		r.execution.conversation.RunID() == "" && r.execution.opening == nil {
		err = errors.Join(err, r.execution.conversation.StopStarting())
	}
	r.transcript().rejectLivePresentation()
	r.transcript().Append(presentError(r.transcript().theme, err.Error()))
	r.header.SetUsage(r.execution.conversation.Usage())
	blocked := r.runAdmissionBlocked()
	r.execution.observation = observationDetached
	if r.execution.conversation.Busy() {
		r.execution.observation = observationFailed
	}
	r.prompt.SetBusy(blocked)
	r.status.fail(err.Error(), blocked)
	r.syncAnimation()
	r.link.raiseAttention(failureAttention())
}

func (r *runs) dropStream() {
	r.tasks.operations.Cancel(streamOperation)
	r.execution.stopFollowing()
}

func (r *runs) startFollowing(work func(context.Context, operationLease)) {
	r.dropStream()
	r.execution.observation = observationFollowing
	if r.tasks.operations.Go(streamOperation, work) {
		return
	}
	r.execution.stopFollowing()
	r.fail(errStreamFollowerUnavailable)
}

func (r *runs) syncAnimation() {
	running := r.execution.conversation.Phase() == conversation.Running && r.execution.following()
	switch {
	case running && r.execution.stopClock == nil:
		r.execution.stopClock = r.every(animationInterval, func() {
			r.status.tick(r.execution.clock.elapsed(time.Now()))
		})
	case !running && r.execution.stopClock != nil:
		r.execution.stopClock()
		r.execution.stopClock = nil
	}
	state := term.Progress{}
	if running {
		state.State = term.ProgressIndeterminate
	}
	r.setProgress(state)
}
