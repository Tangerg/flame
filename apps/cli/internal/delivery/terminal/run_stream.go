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

// startRunCallError identifies an error returned by Lifecycle.StartRun
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

func (r *runs) startRun(commandID authoring.CommandID, message authoring.Message, options authoring.RunOptions, status string) bool {
	input := authoring.StartRun{CommandID: commandID, SessionID: r.session.current.ID, Message: message.Clone(), Options: options.Clone()}
	pending, ok := pendingRunByCommandID(r.workbench.PendingRuns(input.SessionID), input.CommandID)
	if !ok {
		r.fail(errors.New("run is absent from the durable outbox"))
		return false
	}
	if pending.State != workbench.PendingRunQueued {
		return r.startPreparedRun(input, nil, status)
	}
	// The queue reservation owns this candidate while the existing operation
	// owner reads and saves its bytes. Only the UI callback can publish dispatch:
	// canceling preparation therefore leaves an ordinary editable queue entry.
	started := r.tasks.sessionFence(inputPreparationOperation, false,
		func(ctx context.Context) (*workbench.PreparedInput, error) {
			blocks, err := r.runtime.PrepareInput(ctx, pending.Command.Message)
			if err != nil {
				return nil, err
			}
			return r.workbench.PrepareInput(ctx, pending.Command.Message, blocks)
		},
		func(prepared *workbench.PreparedInput, err error) {
			if err == nil && r.startPreparedRun(input, prepared, status) {
				return
			}
			r.queue.ReleaseDispatch(input.SessionID)
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

func (r *runs) startPreparedRun(input authoring.StartRun, prepared *workbench.PreparedInput, status string) bool {
	if !r.prepareRunStart(&input, prepared) {
		return false
	}
	pending, ok := pendingRunByCommandID(r.workbench.PendingRuns(input.SessionID), input.CommandID)
	if !ok {
		r.fail(errors.New("dispatching run disappeared from the durable outbox"))
		return false
	}
	r.presentRunStart(status)
	r.followOpening(func(ctx context.Context) (conversation.SegmentStream, error) {
		opened, err := runworkflow.OpenPendingStart(ctx, r.runtime, r.workbench, pending, r.replayPolicy)
		if err != nil {
			if _, accepted := conversation.AcceptedMutationReceipt(err); accepted {
				return conversation.SegmentStream{}, err
			}
			return conversation.SegmentStream{}, &startRunCallError{err: err}
		}
		if err := opened.ValidateStart(); err != nil {
			return conversation.SegmentStream{}, conversation.NewAcceptedMutationError(opened, fmt.Errorf("start run: %w", err))
		}
		return opened, nil
	}, streamOpeningObserver{
		persistent: true,
		accepted: func(opened conversation.SegmentStream) streamOpeningDisposition {
			r.acceptStartedRun(input, opened)
			return followOpenedStream
		},
		rejected: func(err error) error {
			if _, retired := errors.AsType[*conversation.RetiredOpeningError](err); retired {
				return errors.Join(err, r.retireQueuedCommand(input.SessionID, input.CommandID))
			}
			if receipt, accepted := conversation.AcceptedMutationReceipt(err); accepted {
				if identityErr := protocol.ValidateRunID(receipt.RunID); identityErr != nil {
					return errors.Join(err, identityErr, r.requeueDefinitivelyRefusedStart(input, err))
				}
				r.execution.opening = &openingReceipt{RunID: receipt.RunID, CommandID: input.CommandID}
				r.cancelRuntimePreservingFailure(conversation.CancelRun{
					RunID: receipt.RunID, Reason: "runtime returned an invalid start receipt",
				})
			}
			return errors.Join(err, r.requeueDefinitivelyRefusedStart(input, err))
		},
	})
	return true
}

func (r *runs) prepareRunStart(input *authoring.StartRun, prepared *workbench.PreparedInput) bool {
	pending, ok := pendingRunByCommandID(r.workbench.PendingRuns(input.SessionID), input.CommandID)
	if !ok {
		r.fail(errors.New("run is absent from the durable outbox"))
		return false
	}
	*input = pending.Command.Clone()
	if err := r.execution.conversation.Starting(); err != nil {
		r.fail(err)
		return false
	}
	if pending.State != workbench.PendingRunQueued {
		return true
	}
	replayGuard, err := r.replayPolicy.NewGuard()
	if err != nil {
		r.fail(errors.Join(err, r.execution.conversation.StopStarting()))
		return false
	}
	if err := r.workbench.MarkPendingRunDispatching(input.SessionID, input.CommandID, replayGuard, prepared); err != nil {
		rollbackErr := r.execution.conversation.StopStarting()
		r.link.message("run start blocked: save dispatching run: " + err.Error())
		if rollbackErr != nil {
			r.fail(errors.Join(err, rollbackErr))
		}
		return false
	}
	pending, ok = pendingRunByCommandID(r.workbench.PendingRuns(input.SessionID), input.CommandID)
	if !ok {
		r.fail(errors.New("dispatching run disappeared from the durable outbox"))
		return false
	}
	*input = pending.Command.Clone()
	return true
}

func (r *runs) presentRunStart(status string) {
	r.execution.projectionFailed = false
	r.transcript().Follow()
	r.activity.Reset()
	r.header.SetUsage(conversation.Usage{})
	r.prompt.SetBusy(true)
	r.status.beginRun(status)
	r.execution.clock.start(0, time.Now())
	r.syncAnimation()
}

func (r *runs) acceptStartedRun(input authoring.StartRun, opened conversation.SegmentStream) {
	r.execution.opening = &openingReceipt{RunID: opened.RunID, CommandID: input.CommandID}
	pending := r.workbench.PendingRuns(input.SessionID)
	if len(pending) == 0 || pending[0].Command.CommandID != input.CommandID {
		return
	}
	if pending[0].State != workbench.PendingRunCanceling {
		r.settleQueuedDispatch(input.CommandID)
		return
	}
	r.status.active("canceling")
	r.requestRuntimeCancellation(conversation.CancelRun{
		CommandID: pending[0].CancelCommandID,
		RunID:     opened.RunID,
		Reason:    unconfirmedStartCancellationReason,
	}, applyRuntimeSettlement)
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
	// accepted owns the linearization boundary between command acknowledgement
	// and stream consumption. It may reject a valid runtime stream when the local
	// projection cannot safely install the acknowledged state.
	accepted func(conversation.SegmentStream) streamOpeningDisposition
	rejected func(error) error
	// persistent makes retryable opening failures wait for either an
	// acknowledgement or owner cancellation. It is reserved for idempotent
	// mutations whose delivery outcome is ambiguous after a disconnect.
	persistent bool
}

func (r *runs) followOpening(
	open func(context.Context) (conversation.SegmentStream, error),
	observer streamOpeningObserver,
) {
	sessionID := r.session.current.ID
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
			r.observeCurrentRunStatus()
			if r.execution.opening != nil && r.execution.opening.RunID == event.Run.ID && !r.tasks.operations.Active(pendingRunSettlementOperation) {
				r.execution.opening = nil
			}
			if r.queuedDispatchCanceling() {
				r.status.active("canceling")
			} else if r.tasks.operations.Active(pendingRunSettlementOperation) {
				r.status.note("working · retrying local settlement")
			} else {
				r.status.active("working")
			}
			r.execution.clock.start(event.Run.Usage.Duration, time.Now())
		}
	case conversation.BlockStarted:
		r.noteBlockStarted(event.Block)
	case conversation.BlockCompleted:
		if event.Block.Kind == conversation.BlockTool {
			r.status.active("working")
		}
	case conversation.PlanChanged:
		r.activity.Set(r.execution.conversation.PlanItems())
	case conversation.RunProgress:
		if envelope.RunID == r.execution.conversation.RunID() {
			r.header.SetUsage(r.execution.conversation.Usage())
			r.observeCurrentRunStatus()
			r.status.progress(event)
		} else if event.Activity != nil {
			r.status.active("subagent · " + activityLabel(event.Activity))
		}
	case conversation.SegmentFinished:
		switch {
		case event.Run.Status == protocol.RunStatusFinished:
			if envelope.RunID == r.execution.conversation.RunID() {
				r.noteRunFinished()
			}
		case r.execution.conversation.Phase() == conversation.Waiting:
			r.openInterrupts(r.execution.conversation.Interrupts())
			r.header.SetUsage(r.execution.conversation.Usage())
			r.observeCurrentRunStatus()
			r.status.note(statusWaitingForAnswers)
		}
	case conversation.BlockDelta, conversation.ToolArgumentsDelta, conversation.CustomEvent:
	default:
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
	r.execution.following = false
	r.execution.projectionFailed = false
	r.link.refreshOpenTimeline()
	if r.session.invalidated {
		r.sessions.refreshInvalidatedSession(true)
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
		r.status.note("canceling")
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
	r.execution.following = false
	r.prompts.dismiss()
	if r.execution.conversation.Phase() == conversation.Running &&
		r.execution.conversation.RunID() == "" && r.execution.opening == nil {
		err = errors.Join(err, r.execution.conversation.StopStarting())
	}
	r.transcript().rejectLivePresentation()
	r.transcript().Append(presentError(r.transcript().theme, err.Error()))
	r.header.SetUsage(r.execution.conversation.Usage())
	blocked := r.runAdmissionBlocked()
	r.execution.projectionFailed = r.execution.conversation.Busy()
	r.prompt.SetBusy(blocked)
	r.status.fail(err.Error(), blocked)
	r.syncAnimation()
	r.link.raiseAttention(failureAttention())
}

func (r *runs) dropStream() {
	r.tasks.operations.Cancel(streamOperation)
	r.execution.following = false
}

func (r *runs) startFollowing(work func(context.Context, operationLease)) {
	r.dropStream()
	r.execution.following = true
	if r.tasks.operations.Go(streamOperation, false, work) {
		return
	}
	r.execution.following = false
	r.fail(errStreamFollowerUnavailable)
}

func (r *runs) syncAnimation() {
	running := r.execution.conversation.Phase() == conversation.Running && r.execution.following
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
