package terminal

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Tangerg/flame/cli/internal/application/mutation"
	"github.com/Tangerg/flame/cli/internal/application/workbench"
	"github.com/Tangerg/flame/cli/internal/domain/authoring/prompt"
	"github.com/Tangerg/flame/cli/internal/domain/authoring/replay"
	"github.com/Tangerg/flame/cli/internal/domain/conversation"
	runtimeprotocol "github.com/Tangerg/flame/runtime/protocol"
	"github.com/Tangerg/oolong/core/term"
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

func (a *app) startRun(commandID replay.CommandID, message prompt.Message, options prompt.RunOptions, status string) bool {
	input := prompt.StartRun{CommandID: commandID, SessionID: a.session.current.ID, Message: message.Clone(), Options: options.Clone()}
	pending, ok := pendingRunByCommandID(a.workbench.PendingRuns(input.SessionID), input.CommandID)
	if !ok {
		a.fail(errors.New("run is absent from the durable outbox"))
		return false
	}
	if pending.State != workbench.PendingRunQueued {
		return a.startPreparedRun(input, nil, status)
	}
	// The queue reservation owns this candidate while the existing operation
	// owner reads and saves its bytes. Only the UI callback can publish dispatch:
	// canceling preparation therefore leaves an ordinary editable queue entry.
	started := a.runSessionAdmissionFence(inputPreparationOperation, false,
		func(ctx context.Context) (*workbench.PreparedInput, error) {
			blocks, err := a.runtime.PrepareInput(ctx, pending.Command.Message)
			if err != nil {
				return nil, err
			}
			return a.workbench.PrepareInput(ctx, pending.Command.Message, blocks)
		},
		func(prepared *workbench.PreparedInput, err error) {
			if err == nil && a.startPreparedRun(input, prepared, status) {
				return
			}
			a.queue.ReleaseDispatch(input.SessionID)
			a.syncQueue()
			a.prompt.SetBusy(a.runAdmissionBlocked())
			if err != nil {
				a.message("run start blocked: prepare input: " + err.Error())
			}
		},
	)
	if started {
		a.prompt.SetBusy(true)
		a.status.note("preparing input")
	}
	return started
}

func (a *app) startPreparedRun(input prompt.StartRun, prepared *workbench.PreparedInput, status string) bool {
	replayGuard, ready := a.prepareRunStart(&input, prepared)
	if !ready {
		return false
	}
	a.presentRunStart(status)
	a.followOpening(func(ctx context.Context) (conversation.SegmentStream, error) {
		if err := mutation.FreshReplayAdmission(a.replayPolicy, replayGuard)(); err != nil {
			return conversation.SegmentStream{}, err
		}
		opened, err := a.runtime.StartRun(ctx, input)
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
			a.acceptStartedRun(input, opened)
			return followOpenedStream
		},
		rejected: func(err error) error {
			if receipt, accepted := conversation.AcceptedMutationReceipt(err); accepted {
				if identityErr := runtimeprotocol.ValidateRunID(receipt.RunID); identityErr != nil {
					return errors.Join(err, identityErr, a.requeueDefinitivelyRefusedStart(input, err))
				}
				a.execution.openingRunID = receipt.RunID
				a.cancelRuntimePreservingFailure(conversation.CancelRun{
					RunID: receipt.RunID, Reason: "runtime returned an invalid start receipt",
				})
			}
			return errors.Join(err, a.requeueDefinitivelyRefusedStart(input, err))
		},
	})
	return true
}

func (a *app) prepareRunStart(input *prompt.StartRun, prepared *workbench.PreparedInput) (replay.Guard, bool) {
	pending, ok := pendingRunByCommandID(a.workbench.PendingRuns(input.SessionID), input.CommandID)
	if !ok {
		a.fail(errors.New("run is absent from the durable outbox"))
		return replay.Guard{}, false
	}
	if pending.State != workbench.PendingRunQueued {
		command, err := pending.ReplayCommand()
		if err != nil {
			a.fail(fmt.Errorf("recover pending run input: %w", err))
			return replay.Guard{}, false
		}
		*input = command
	}
	if err := a.execution.conversation.Starting(); err != nil {
		a.fail(err)
		return replay.Guard{}, false
	}
	replayGuard, err := a.replayPolicy.NewGuard()
	if err != nil {
		a.fail(errors.Join(err, a.execution.conversation.CancelStarting()))
		return replay.Guard{}, false
	}
	if err := a.workbench.MarkPendingRunDispatching(input.SessionID, input.CommandID, replayGuard, prepared); err != nil {
		rollbackErr := a.execution.conversation.CancelStarting()
		a.message("run start blocked: save dispatching run: " + err.Error())
		if rollbackErr != nil {
			a.fail(errors.Join(err, rollbackErr))
		}
		return replay.Guard{}, false
	}
	pending, ok = pendingRunByCommandID(a.workbench.PendingRuns(input.SessionID), input.CommandID)
	if !ok {
		a.fail(errors.New("dispatching run disappeared from the durable outbox"))
		return replay.Guard{}, false
	}
	*input = pending.Command.Clone()
	return pending.Replay, true
}

func (a *app) presentRunStart(status string) {
	a.execution.projectionFailed = false
	a.transcript.Follow()
	a.activity.Reset()
	a.header.SetUsage(conversation.Usage{})
	a.prompt.SetBusy(true)
	a.status.beginRun(status)
	a.execution.clock.start(0, time.Now())
	a.syncAnimation()
}

func (a *app) acceptStartedRun(input prompt.StartRun, opened conversation.SegmentStream) {
	a.execution.openingRunID = opened.RunID
	pending := a.workbench.PendingRuns(input.SessionID)
	if len(pending) == 0 || pending[0].Command.CommandID != input.CommandID {
		return
	}
	if pending[0].State != workbench.PendingRunCanceling {
		return
	}
	a.status.active("canceling")
	a.requestRuntimeCancellation(conversation.CancelRun{
		CommandID: pending[0].CancelCommandID,
		RunID:     opened.RunID,
		Reason:    unconfirmedStartCancellationReason,
	}, applyRuntimeSettlement)
}

func (a *app) requeueDefinitivelyRefusedStart(input prompt.StartRun, failure error) error {
	callFailure, refused := errors.AsType[*startRunCallError](failure)
	_, dispatchingPresent := a.queue.Dispatching(input.SessionID)
	if !refused || mutation.OutcomeUnknown(callFailure.err) || !dispatchingPresent {
		return nil
	}
	if err := a.queue.RequeueDispatch(input.SessionID, input.CommandID); err != nil {
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

func (a *app) followOpening(
	open func(context.Context) (conversation.SegmentStream, error),
	observer streamOpeningObserver,
) {
	sessionID := a.session.current.ID
	a.startFollowing(func(ctx context.Context, lease operationLease) {
		follower := streamFollower{
			app: a, ctx: ctx, dispatcher: a.loop.Dispatcher(), lease: lease, sessionID: sessionID,
			open: open, applyEvent: a.apply,
		}
		follower.opening = observer
		follower.run()
	})
}

func (a *app) apply(event conversation.RunEvent) error {
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
	a.observeSteerEvent(event)
	a.status.setRunningDescendants(a.execution.conversation.RunningDescendants())
	switch event.Event.(type) {
	case conversation.SegmentStarted, conversation.RunProgress, conversation.RunInterrupted, conversation.RunSuspended, conversation.RunFinished:
		a.refreshOpenTimeline()
	}
	a.transcript.DiscardExcess()
	a.syncAnimation()
	return nil
}

func (a *app) applyPresentationEvent(envelope conversation.RunEvent) {
	switch event := envelope.Event.(type) {
	case conversation.SegmentStarted:
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
	case conversation.BlockStarted:
		a.noteBlockStarted(event.Block)
	case conversation.BlockCompleted:
		if event.Block.Kind == conversation.BlockTool {
			a.status.active("working")
		}
	case conversation.PlanChanged:
		a.activity.Set(a.execution.conversation.PlanItems())
	case conversation.RunProgress:
		if envelope.RunID == a.execution.conversation.RunID() {
			a.header.SetUsage(a.execution.conversation.Usage())
			a.observeCurrentRunStatus()
			a.status.progress(event)
		} else if strings.TrimSpace(event.Activity) != "" {
			a.status.active("subagent · " + event.Activity)
		}
	case conversation.RunInterrupted:
		if a.execution.conversation.Phase() == conversation.Waiting {
			a.openInterrupts(a.execution.conversation.Interrupts())
			a.header.SetUsage(a.execution.conversation.Usage())
			a.observeCurrentRunStatus()
			a.status.note("waiting for your answers")
		}
	case conversation.RunSuspended:
		if a.execution.conversation.Phase() == conversation.Waiting {
			a.openInterrupts(a.execution.conversation.Interrupts())
			a.header.SetUsage(a.execution.conversation.Usage())
			a.observeCurrentRunStatus()
			a.status.note("waiting for your answers")
		}
	case conversation.RunFinished:
		if envelope.RunID == a.execution.conversation.RunID() {
			a.noteRunFinished()
		}
	case conversation.BlockDelta, conversation.ToolArgumentsDelta, conversation.CustomEvent:
	default:
	}
}

func (a *app) noteBlockStarted(block conversation.Block) {
	if block.Kind == conversation.BlockTool && block.Tool != nil {
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
	if a.execution.conversation.Phase() != conversation.Idle || a.execution.conversation.Outcome().Status == "" {
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
	a.refreshSteerPresentation()
	a.raiseAttention(outcomeAttention(a.execution.conversation.Outcome()))
}

func outcomeNotification(outcome conversation.Outcome) string {
	switch outcome.Status {
	case runtimeprotocol.OutcomeCompleted:
		return "flame run completed"
	case runtimeprotocol.OutcomeCanceled:
		return "flame run canceled"
	case runtimeprotocol.OutcomeTimedOut:
		return "flame run stopped: " + string(outcome.Status)
	case runtimeprotocol.OutcomeFailed, runtimeprotocol.OutcomeLost:
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
	a.dismissInterruptProjection()
	if a.execution.conversation.Phase() == conversation.Running &&
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
	running := a.execution.conversation.Phase() == conversation.Running && a.execution.following
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
