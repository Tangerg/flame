package terminal

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/flame/apps/cli/internal/application/mutation"
	"github.com/Tangerg/flame/apps/cli/internal/application/workbench"
	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/oolong/core/term"
)

// runLink is what Run execution asks of the rest of the terminal: notices,
// attention, the authoritative Session refresh and the open timeline.
type runLink interface {
	message(label string)
	terminalClosed() bool
	dismissCompletion()
	raiseAttention(signal attentionSignal)
	refreshOpenTimeline()
}

// runSession is what Run execution asks of the Session lifecycle: re-read the
// presented Session, or replace its projection after a replay gap.
type runSession interface {
	refreshInvalidatedSession(settlementFence bool)
	reconcileRunSnapshot(snapshot conversation.SessionSnapshot, stream conversation.SegmentStream) error
}

// runs owns the observed Run: its conversation projection, the start and
// cancellation it has in flight, queue dispatch into Run admission, stream
// following and recovery, and the interrupt resume. Other surfaces read the
// conversation through runs and never change it.
type runs struct {
	execution executionState

	runtime      Runtime
	workbench    *workbench.Store
	queue        *workbench.Queue
	replayPolicy mutation.ReplayPolicy
	session      *sessionState
	tasks        taskRunner
	every        func(time.Duration, func()) func()
	setProgress  func(term.Progress)
	report       func(workbenchConcern, error)
	link         runLink
	sessions     runSession

	transcript  func() *transcriptView
	status      *statusView
	prompt      *promptView
	header      *sessionHeader
	activity    *activityView
	prompts     *interruptPrompts
	steering    *steerDesk
	queueEditor *queueEditor
	composer    *promptComposer
}

func (r *runs) message(label string) { r.link.message(label) }

func (r *runs) conversation() *conversation.Conversation { return r.execution.conversation }

func (r *runs) observing() bool { return r.execution.observing() }

func (r *runs) blocksAdmission() bool { return r.execution.blocksAdmission() }

func (r *runs) following() bool { return r.execution.following }

func (r *runs) cancelPending() bool { return r.execution.pendingCancel != nil }

// adoptProjection makes conversation the observed projection, as a snapshot
// reconciliation does for the same Session.
func (r *runs) adoptProjection(conversation *conversation.Conversation) {
	r.execution.conversation = conversation
	r.execution.projectionFailed = false
}

// switchProjection adopts another Session's projection; no start of the
// previous Session is still being opened.
func (r *runs) switchProjection(conversation *conversation.Conversation) {
	r.execution.opening = nil
	r.adoptProjection(conversation)
}

func (r *runs) stopClock() {
	if r.execution.stopClock != nil {
		r.execution.stopClock()
		r.execution.stopClock = nil
	}
}

// runShutdown is the cancellation a closing terminal still owes.
type runShutdown struct {
	target        conversation.CancelRun
	opening       authoring.CommandID
	cancelRuntime bool
	replay        authoring.ReplayGuard
	pendingStart  workbench.PendingRun
	cancelOpening bool
}

// settleOpeningForShutdown settles an acknowledged start before the terminal
// closes, so its queue entry does not outlive the process.
func (r *runs) settleOpeningForShutdown() error {
	if r.execution.blocksAdmission() || r.execution.opening == nil {
		return nil
	}
	if err := r.attemptQueuedDispatchSettlement(r.execution.opening.CommandID); err != nil {
		return fmt.Errorf("settle acknowledged run start: %w", err)
	}
	r.execution.opening = nil
	return nil
}

// planShutdown decides which Run or start the closing terminal cancels. A
// detached terminal leaves its Run alive and only finishes a cancellation the
// user already asked for; stageable reports whether a start may be staged.
func (r *runs) planShutdown(detach, stageable bool) (runShutdown, error) {
	var plan runShutdown
	var planErr error
	if pending := r.execution.pendingCancel; pending != nil {
		plan.target, plan.opening, plan.cancelRuntime, plan.replay = pending.request, pending.openingCommandID, true, pending.replay
	} else if !detach {
		plan.target, plan.cancelRuntime = r.activeCancellation()
		plan.opening = r.openingCommandForRun(plan.target.RunID)
		replay, err := r.replayPolicy.NewGuard()
		if err != nil {
			planErr = err
			plan.cancelRuntime = false
		}
		plan.replay = replay
	}
	if plan.cancelRuntime || !stageable || planErr != nil {
		return plan, planErr
	}
	if detach {
		for _, pending := range r.workbench.PendingRuns(r.session.current.ID) {
			if pending.State == workbench.PendingRunCanceling {
				plan.pendingStart, plan.cancelOpening = pending, true
				break
			}
		}
		return plan, nil
	}
	var err error
	plan.pendingStart, plan.cancelOpening, err = r.stageOpeningCancellation()
	return plan, err
}

func (r *runs) finishShutdown(ctx context.Context, plan runShutdown) error {
	switch {
	case plan.cancelRuntime:
		if err := r.cancelRuntimeNow(ctx, plan.target, plan.replay); err != nil {
			return err
		}
		return r.retireCanceledRuntimeOwnership(plan.target.RunID, plan.opening)
	case plan.cancelOpening:
		return r.cancelOpeningRunNow(ctx, plan.pendingStart)
	default:
		return nil
	}
}

// resumeReconciled continues a reconciled projection in its phase. It reports
// false when Idle dispatched the next queued prompt, which owns what follows.
func (r *runs) resumeReconciled(snapshot conversation.SessionSnapshot) (bool, error) {
	switch r.execution.conversation.Phase() {
	case conversation.Running:
		r.execution.following = true
		r.execution.clock.start(r.execution.conversation.Usage().Duration, time.Now())
		active, ok := snapshot.ActiveRun()
		if !ok {
			return false, errors.New("reconcile run snapshot: running conversation has no active run")
		}
		r.showRecoveredRunStatus("reconnected", active)
	case conversation.Waiting:
		r.execution.following = false
		if r.prompts.review == nil {
			r.openInterrupts(r.execution.conversation.Interrupts())
		}
		r.observeCurrentRunStatus()
		r.status.note(statusWaitingForAnswers)
	case conversation.Idle:
		r.execution.following = false
		if r.execution.conversation.Outcome().Status != "" {
			r.settleCurrentRunStatus()
		}
		if r.drainQueue() {
			return false, nil
		}
		if r.execution.conversation.Outcome().Status != "" {
			r.link.raiseAttention(outcomeAttention(r.execution.conversation.Outcome()))
		}
	default:
		return false, errors.New("reconcile run snapshot: unknown conversation phase")
	}
	return true, nil
}

// resumeRestored continues a projection restored at startup in its phase.
func (r *runs) resumeRestored(snapshot conversation.SessionSnapshot) {
	switch r.execution.conversation.Phase() {
	case conversation.Waiting:
		if r.prompts.review == nil {
			r.openInterrupts(r.execution.conversation.Interrupts())
		}
		r.observeCurrentRunStatus()
		r.status.note(statusWaitingForAnswers)
	case conversation.Running:
		active, ok := snapshot.ActiveRun()
		if !ok {
			r.fail(errors.New("session snapshot has a running conversation without an active run"))
			return
		}
		r.execution.clock.start(r.execution.conversation.Usage().Duration, time.Now())
		r.showRecoveredRunStatus("reconnecting", active)
		r.followRecoveredSession()
	case conversation.Idle:
		if r.execution.conversation.Outcome().Status != "" {
			r.settleCurrentRunStatus()
		}
	default:
		r.fail(errors.New("session snapshot has an unknown conversation phase"))
	}
}

func (r *runs) showRecoveredRunStatus(label string, run conversation.Run) {
	r.status.observeRun(run)
	r.status.active(label)
}
