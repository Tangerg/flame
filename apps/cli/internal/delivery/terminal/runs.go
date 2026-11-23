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
	"github.com/Tangerg/oolong/ui/core/term"
)

type runLink interface {
	message(label string)
	terminalClosed() bool
	dismissCompletion()
	raiseAttention(signal attentionSignal)
	refreshOpenTimeline()
}

type runSession interface {
	refreshInvalidatedSession(sessionRefresh)
	reconcileRunSnapshot(snapshot conversation.SessionSnapshot, stream conversation.SegmentStream) error
}

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

func (r *runs) following() bool { return r.execution.following() }

func (r *runs) restoreSnapshot(snapshot conversation.SessionSnapshot) error {
	return r.execution.conversation.RestoreSnapshot(snapshot)
}

func (r *runs) clearPresentation() { r.execution.conversation.ClearPresentation() }

func (r *runs) cancelPending() bool { return r.execution.pendingCancel != nil }

func (r *runs) adoptProjection(conversation *conversation.Conversation) {
	r.execution.conversation = conversation
	r.execution.trustProjection()
}

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

type shutdownCancellation uint8

const (
	shutdownOwesNothing shutdownCancellation = iota
	shutdownCancelsRun
	shutdownCancelsStart
)

type runShutdown struct {
	owes         shutdownCancellation
	target       conversation.CancelRun
	opening      authoring.CommandID
	replay       authoring.ReplayGuard
	pendingStart workbench.PendingRun
}

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

func (r *runs) planShutdown(detach, stageable bool) (runShutdown, error) {
	if pending := r.execution.pendingCancel; pending != nil {
		return runShutdown{owes: shutdownCancelsRun, target: pending.request, opening: pending.openingCommandID, replay: pending.replay}, nil
	}
	if !detach {
		if target, active := r.activeCancellation(); active {
			replay, err := r.replayPolicy.NewGuard()
			if err != nil {
				return runShutdown{}, err
			}
			return runShutdown{owes: shutdownCancelsRun, target: target, opening: r.openingCommandForRun(target.RunID), replay: replay}, nil
		}
	}
	if !stageable {
		return runShutdown{}, nil
	}
	if detach {
		for _, pending := range r.workbench.PendingRuns(r.session.current.ID) {
			if pending.State == workbench.PendingRunCanceling {
				return runShutdown{owes: shutdownCancelsStart, pendingStart: pending}, nil
			}
		}
		return runShutdown{}, nil
	}
	pending, staged, err := r.stageOpeningCancellation()
	if err != nil || !staged {
		return runShutdown{}, err
	}
	return runShutdown{owes: shutdownCancelsStart, pendingStart: pending}, nil
}

func (r *runs) finishShutdown(ctx context.Context, plan runShutdown) error {
	switch plan.owes {
	case shutdownCancelsRun:
		if err := r.cancelRuntimeNow(ctx, plan.target, plan.replay); err != nil {
			return err
		}
		return r.retireCanceledRuntimeOwnership(plan.target.RunID, plan.opening)
	case shutdownCancelsStart:
		return r.cancelOpeningRunNow(ctx, plan.pendingStart)
	default:
		return nil
	}
}

func (r *runs) resumeReconciled(snapshot conversation.SessionSnapshot) (bool, error) {
	switch r.execution.conversation.Phase() {
	case conversation.Running:
		r.execution.observation = observationFollowing
		r.execution.clock.start(r.execution.conversation.Usage().Duration, time.Now())
		active, ok := snapshot.ActiveRun()
		if !ok {
			return false, errors.New("reconcile run snapshot: running conversation has no active run")
		}
		r.showRecoveredRunStatus("reconnected", active)
	case conversation.Waiting:
		r.execution.stopFollowing()
		if r.prompts.review == nil {
			r.openInterrupts(r.execution.conversation.Interrupts())
		}
		r.observeCurrentRunStatus()
		r.status.note(statusWaitingForAnswers)
	case conversation.Idle:
		r.execution.stopFollowing()
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
