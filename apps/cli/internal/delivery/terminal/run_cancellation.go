package terminal

import (
	"context"
	"errors"
	"fmt"
	"time"

	runworkflow "github.com/Tangerg/flame/apps/cli/internal/application/agent/run"
	"github.com/Tangerg/flame/apps/cli/internal/application/mutation"
	"github.com/Tangerg/flame/apps/cli/internal/application/retry"
	"github.com/Tangerg/flame/apps/cli/internal/application/workbench"
	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
)

const (
	runtimeControlTimeout              = 5 * time.Second
	unconfirmedStartCancellationReason = "canceled while start delivery was unconfirmed"
)

func (r *runs) cancel() {
	if r.prompts.decline() {
		return
	}
	if r.tasks.operations.Active(inputPreparationOperation) {
		r.tasks.operations.Cancel(inputPreparationOperation)
		r.queue.ReleasePreparation(r.session.current.ID)
		r.syncQueue()
		r.prompt.SetBusy(r.runAdmissionBlocked())
		r.link.message("input preparation canceled; the prompt remains editable")
		return
	}
	if r.execution.pendingCancel != nil {
		r.status.doing = "retrying cancellation"
		r.cancelRuntime(r.execution.pendingCancel.request)
		return
	}
	if !r.execution.conversation.Busy() && !r.execution.following() {
		return
	}
	r.status.doing = statusCanceling
	runID := r.execution.conversation.RunID()
	if runID == "" {
		pending, staged, err := r.stageOpeningCancellation()
		if err != nil {
			r.link.message("could not preserve cancellation of unconfirmed run: " + err.Error())
			return
		}
		if !staged {
			return
		}
		r.dropStream()
		if err := r.execution.conversation.StopStarting(); err != nil {
			r.fail(err)
			return
		}
		r.prompt.SetBusy(false)
		r.status.note("canceled · reconciling runtime delivery")
		r.syncAnimation()
		r.reconcileCanceledStart(pending)
		return
	}
	r.cancelRuntime(conversation.CancelRun{RunID: runID, Reason: "canceled by the terminal user"})
}

func (r *runs) stageOpeningCancellation() (workbench.PendingRun, bool, error) {
	sessionID := r.session.current.ID
	head, found := r.queue.Head(sessionID)
	if !found || head.Dispatch == nil {
		return workbench.PendingRun{}, false, nil
	}
	if head.Cancellation() != nil {
		return head, true, nil
	}
	replay, err := r.replayPolicy.NewGuard()
	if err != nil {
		return workbench.PendingRun{}, false, err
	}
	pending, err := r.queue.StageCancellation(sessionID, head.Command.CommandID, replay)
	return pending, err == nil, err
}

func (r *runs) reconcileCanceledStart(pending workbench.PendingRun) {
	r.tasks.operations.GoSession(pendingRunRecoveryOperation, func(ctx context.Context, lease operationLease) {
		opened, err := openStartRunWithBackoff(
			ctx, r.runtime, r.workbench, pending, r.replayPolicy, runtimeRecoveryBackoff,
		)
		if context.Cause(ctx) != nil {
			return
		}
		_ = r.tasks.deliver(ctx, func() {
			if !r.tasks.operations.Current(lease) || r.link.terminalClosed() || r.session.current.ID != pending.Command.SessionID ||
				!r.tasks.operations.Release(lease) {
				return
			}
			if errors.Is(err, conversation.ErrOpeningRetired) {
				if err := r.retireCanceledStart(pending); err != nil {
					r.fail(err)
				}
				return
			}
			if err != nil {
				if mutation.OutcomeUnknown(err) {
					r.fail(fmt.Errorf("reconcile canceled start: %w", err))
					return
				}
				if retireErr := r.retireCanceledStart(pending); retireErr != nil {
					r.fail(errors.Join(err, retireErr))
				}
				return
			}
			r.cancelRuntime(conversation.CancelRun{
				CommandID: pending.Cancellation().CommandID,
				RunID:     opened.RunID(),
				Reason:    unconfirmedStartCancellationReason,
			})
		})
	})
}

func openStartRunWithBackoff(
	ctx context.Context,
	runtime runworkflow.OpeningSource,
	store *workbench.Store,
	pending workbench.PendingRun,
	replayPolicy mutation.ReplayPolicy,
	backoff retry.Backoff,
) (runworkflow.Opening, error) {
	return mutation.ConfirmAdmitted(
		ctx, backoff, nil,
		func(ctx context.Context) (runworkflow.Opening, error) {
			return runworkflow.OpenPendingStart(ctx, runtime, store, pending, replayPolicy)
		},
	)
}

func (r *runs) retireCanceledStart(pending workbench.PendingRun) error {
	if err := r.retireQueuedCommand(pending.Command.SessionID, pending.Command.CommandID); err != nil {
		return fmt.Errorf("retire canceled start: %w", err)
	}
	r.status.note(canceledActivity)
	r.drainQueue()
	return nil
}

func (r *runs) activeCancellation() (conversation.CancelRun, bool) {
	if r.execution.projectionFailed() {
		return conversation.CancelRun{}, false
	}
	if runID := r.execution.conversation.RunID(); runID != "" && r.execution.conversation.Busy() {
		return conversation.CancelRun{RunID: runID, Reason: "terminal closed"}, true
	}
	if r.execution.opening != nil && r.execution.conversation.Busy() {
		return conversation.CancelRun{RunID: r.execution.opening.RunID, Reason: "terminal closed"}, true
	}
	return conversation.CancelRun{}, false
}

type pendingCancellation struct {
	request          conversation.CancelRun
	openingCommandID authoring.CommandID
	replay           authoring.ReplayGuard
}

func (r *runs) cancelRuntime(target conversation.CancelRun) {
	if target.CommandID == "" {
		commandID := mutation.NewCommandID()
		target.CommandID = commandID
	}
	pending := pendingCancellation{
		request: target, openingCommandID: r.openingCommandForRun(target.RunID),
	}
	head, _ := r.queue.Head(r.session.current.ID)
	current := r.execution.pendingCancel
	switch original := head.Cancellation(); {
	case original != nil && original.CommandID == target.CommandID:
		pending.openingCommandID = head.Command.CommandID
		pending.replay = original.Replay
	case current != nil && current.request.CommandID == target.CommandID:
		pending.openingCommandID = current.openingCommandID
		pending.replay = current.replay
	default:
		guard, err := r.replayPolicy.NewGuard()
		if err != nil {
			r.link.message("could not cancel run: " + err.Error())
			return
		}
		pending.replay = guard
	}
	if pending.openingCommandID == "" && current != nil && current.request.RunID == target.RunID {
		pending.openingCommandID = current.openingCommandID
	}

	r.execution.pendingCancel = &pending
	r.tasks.operations.Go(cancelRunOperation, func(ownerCtx context.Context, lease operationLease) {
		err := r.cancelRootRun(ownerCtx, target, pending.replay)
		_ = r.tasks.deliver(ownerCtx, func() {
			r.handleRuntimeCancellation(lease, pending, err)
		})
	})
}

func (r *runs) cancelRootRun(
	ctx context.Context,
	target conversation.CancelRun,
	replayGuard authoring.ReplayGuard,
) error {
	result, err := mutation.ConfirmAdmitted(
		ctx, runtimeRecoveryBackoff, mutation.ReplayAdmission(r.replayPolicy, replayGuard),
		func(ctx context.Context) (conversation.RunCancellation, error) {
			if r.workbench != nil {
				if err := r.workbench.CancellationReady(target.CommandID); err != nil {
					return conversation.RunCancellation{}, err
				}
			}
			attemptCtx, cancel := context.WithTimeout(ctx, runtimeControlTimeout)
			defer cancel()
			return r.runtime.CancelRun(attemptCtx, target)
		},
	)
	if err == nil {
		if validateTargetErr := result.ValidateTarget(target.RunID); validateTargetErr != nil {
			return fmt.Errorf("cancel run: %w", validateTargetErr)
		}
		return nil
	}
	if errors.Is(err, conversation.ErrRunFinished) {
		return nil
	}
	return err
}

func (r *runs) handleRuntimeCancellation(
	lease operationLease,
	pending pendingCancellation,
	err error,
) {
	if !r.tasks.operations.Current(lease) || r.link.terminalClosed() {
		return
	}
	if err != nil {
		r.link.message("could not cancel run: " + err.Error())
		return
	}
	if retireErr := r.retireCanceledRuntimeOwnership(pending.request.RunID, pending.openingCommandID); retireErr != nil {
		failure := fmt.Errorf("retire canceled runtime ownership: %w", retireErr)
		r.report(workbenchCancellationOwnership, failure)
		r.link.message("could not " + failure.Error())
		r.retryCanceledRuntimeOwnership(pending.request.RunID, pending.openingCommandID)
	} else {
		r.report(workbenchCancellationOwnership, nil)
	}
	if current := r.execution.pendingCancel; current != nil && current.request.CommandID == pending.request.CommandID {
		r.execution.pendingCancel = nil
	}
	r.execution.opening = nil
	r.dropStream()
	r.status.note(canceledActivity)
	r.prompt.SetBusy(false)
	r.syncAnimation()
	r.sessions.refreshInvalidatedSession(refreshSettlingRun)
}

func (r *runs) openingCommandForRun(runID string) authoring.CommandID {
	if r.execution.opening == nil || runID != r.execution.opening.RunID {
		return ""
	}
	return r.execution.opening.CommandID
}

func (r *runs) retireCanceledRuntimeOwnership(runID string, openingCommandID authoring.CommandID) error {
	var err error
	if r.workbench != nil {
		if pending, ok := r.workbench.PendingResume(r.session.current.ID); ok && pending.Command.RunID == runID {
			err = errors.Join(err, r.workbench.RetirePendingResume(r.session.current.ID, pending.Command.CommandID))
		}
	}
	if openingCommandID != "" {
		err = errors.Join(err, r.retireQueuedCommand(r.session.current.ID, openingCommandID))
	}
	return err
}

func (r *runs) cancelRuntimeNow(
	ownerCtx context.Context,
	target conversation.CancelRun,
	replayGuard authoring.ReplayGuard,
) error {
	if target.CommandID == "" {
		commandID := mutation.NewCommandID()
		target.CommandID = commandID
		guard, err := r.replayPolicy.NewGuard()
		if err != nil {
			return err
		}
		replayGuard = guard
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ownerCtx), runtimeControlTimeout)
	defer cancel()
	result, err := mutation.ConfirmAdmitted(
		ctx, runtimeRecoveryBackoff, mutation.ReplayAdmission(r.replayPolicy, replayGuard),
		func(ctx context.Context) (conversation.RunCancellation, error) {
			if r.workbench != nil {
				if err := r.workbench.CancellationReady(target.CommandID); err != nil {
					return conversation.RunCancellation{}, err
				}
			}
			return r.runtime.CancelRun(ctx, target)
		},
	)
	if errors.Is(err, conversation.ErrRunFinished) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := result.ValidateTarget(target.RunID); err != nil {
		return fmt.Errorf("validate terminal-close cancellation: %w", err)
	}
	return nil
}

func (r *runs) cancelOpeningRunNow(ownerCtx context.Context, pending workbench.PendingRun) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ownerCtx), runtimeControlTimeout)
	defer cancel()
	opened, err := openStartRunWithBackoff(
		ctx, r.runtime, r.workbench, pending, r.replayPolicy, runtimeRecoveryBackoff,
	)
	if errors.Is(err, conversation.ErrOpeningRetired) {
		return r.retireCanceledStart(pending)
	}
	if err != nil {
		if !mutation.OutcomeUnknown(err) {
			return r.retireCanceledStart(pending)
		}
		return fmt.Errorf("reconcile run start during terminal close: %w", err)
	}
	cancelErr := r.cancelRuntimeNow(ctx, conversation.CancelRun{
		CommandID: pending.Cancellation().CommandID,
		RunID:     opened.RunID(),
		Reason:    unconfirmedStartCancellationReason,
	}, pending.Cancellation().Replay)
	if cancelErr != nil {
		return fmt.Errorf("cancel run opened during terminal close: %w", cancelErr)
	}
	return r.retireCanceledStart(pending)
}
