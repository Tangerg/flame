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
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
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
		if entry, reserved := r.queue.Dispatching(r.session.current.ID); reserved {
			pending, found := pendingRunByCommandID(r.workbench.PendingRuns(r.session.current.ID), entry.CommandID)
			if found && pending.State == workbench.PendingRunQueued {
				r.queue.ReleaseDispatch(r.session.current.ID)
			}
		}
		r.syncQueue()
		r.prompt.SetBusy(r.runAdmissionBlocked())
		r.link.message("input preparation canceled; the prompt remains editable")
		return
	}
	if r.execution.pendingCancel != nil {
		r.status.doing = "retrying cancellation"
		r.requestRuntimeCancellation(r.execution.pendingCancel.request, r.execution.pendingCancel.policy)
		return
	}
	if !r.execution.conversation.Busy() && !r.execution.following {
		return
	}
	r.status.doing = "canceling"
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
	entry, ok := r.queue.Dispatching(r.session.current.ID)
	if !ok {
		return workbench.PendingRun{}, false, nil
	}
	if entry.CommandID == "" {
		return workbench.PendingRun{}, false, errors.New("dispatching queue entry is no longer available")
	}
	if pending, found := pendingRunByCommandID(r.workbench.PendingRuns(r.session.current.ID), entry.CommandID); found &&
		pending.State == workbench.PendingRunQueued {
		// Input preparation has not published a dispatch intent. Closing the
		// operation owner will discard its result without a Runtime mutation.
		return workbench.PendingRun{}, false, nil
	}
	cancelReplay, err := r.replayPolicy.NewGuard()
	if err != nil {
		return workbench.PendingRun{}, false, err
	}
	if _, err := r.workbench.MarkPendingRunCanceling(r.session.current.ID, entry.CommandID, cancelReplay); err != nil {
		return workbench.PendingRun{}, false, err
	}
	pending, ok := pendingRunByCommandID(r.workbench.PendingRuns(r.session.current.ID), entry.CommandID)
	if !ok {
		return workbench.PendingRun{}, false, errors.New("canceling run start disappeared from the durable outbox")
	}
	return pending, true, nil
}

func (r *runs) reconcileCanceledStart(pending workbench.PendingRun) {
	dispatcher := r.tasks.dispatch
	r.tasks.operations.GoSession(pendingRunRecoveryOperation, false, func(ctx context.Context, lease operationLease) {
		opened, err := openStartRunWithBackoff(
			ctx, r.runtime, r.workbench, pending, r.replayPolicy, runtimeRecoveryBackoff,
		)
		if context.Cause(ctx) != nil {
			return
		}
		_ = post(ctx, dispatcher, func() {
			if !r.tasks.operations.Current(lease) || r.link.terminalClosed() || r.session.current.ID != pending.Command.SessionID ||
				!r.tasks.operations.Release(lease) {
				return
			}
			if _, retired := errors.AsType[*conversation.RetiredOpeningError](err); retired {
				if err := r.retireCanceledStart(pending); err != nil {
					r.fail(err)
				}
				return
			}
			observed, accepted := observedSegmentStream(opened, err)
			if !accepted {
				if mutation.OutcomeUnknown(err) {
					r.fail(fmt.Errorf("reconcile canceled start: %w", err))
					return
				}
				if retireErr := r.retireCanceledStart(pending); retireErr != nil {
					r.fail(errors.Join(err, retireErr))
				}
				return
			}
			validationErr := observed.ValidateStart()
			if identityErr := protocol.ValidateRunID(observed.RunID); identityErr != nil {
				r.fail(errors.Join(
					fmt.Errorf("reconcile canceled start: accepted receipt: %w", identityErr),
					err,
					validationErr,
				))
				return
			}
			if receiptErr := errors.Join(err, validationErr); receiptErr != nil {
				r.link.message("runtime returned an invalid start receipt; canceling accepted run: " + receiptErr.Error())
			}
			r.requestRuntimeCancellation(conversation.CancelRun{
				CommandID: pending.CancelCommandID,
				RunID:     observed.RunID,
				Reason:    unconfirmedStartCancellationReason,
			}, recoverCanceledOpening)
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
) (conversation.SegmentStream, error) {
	return mutation.ConfirmAdmitted(
		ctx, backoff, nil,
		func(ctx context.Context) (conversation.SegmentStream, error) {
			return runworkflow.OpenPendingStart(ctx, runtime, store, pending, replayPolicy)
		},
	)
}

func observedSegmentStream(stream conversation.SegmentStream, err error) (conversation.SegmentStream, bool) {
	if err == nil {
		return stream, true
	}
	receipt, accepted := conversation.AcceptedMutationReceipt(err)
	return receipt, accepted
}

func (r *runs) retireCanceledStart(pending workbench.PendingRun) error {
	if err := r.retireQueuedCommand(pending.Command.SessionID, pending.Command.CommandID); err != nil {
		return fmt.Errorf("retire canceled start: %w", err)
	}
	r.status.note("canceled")
	r.drainQueue()
	return nil
}

func (r *runs) activeCancellation() (conversation.CancelRun, bool) {
	if r.execution.projectionFailed {
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

func (r *runs) cancelRuntime(target conversation.CancelRun) {
	r.requestRuntimeCancellation(target, applyRuntimeSettlement)
}

// cancelRuntimePreservingFailure stops a run whose event stream has already been
// rejected locally. The cancellation receipt proves cleanup, but only a cold
// Session read can replace the now-untrustworthy transcript projection.
func (r *runs) cancelRuntimePreservingFailure(target conversation.CancelRun) {
	r.requestRuntimeCancellation(target, recoverProjectionFailure)
}

type cancellationResultPolicy uint8

const (
	applyRuntimeSettlement cancellationResultPolicy = iota
	recoverProjectionFailure
	recoverCanceledOpening
)

type pendingCancellation struct {
	request          conversation.CancelRun
	openingCommandID authoring.CommandID
	policy           cancellationResultPolicy
	replay           authoring.ReplayGuard
}

func (r *runs) requestRuntimeCancellation(target conversation.CancelRun, policy cancellationResultPolicy) {
	if target.CommandID == "" {
		commandID := mutation.NewCommandID()
		target.CommandID = commandID
	}
	var replayGuard authoring.ReplayGuard
	if current := r.execution.pendingCancel; current != nil && current.request.CommandID == target.CommandID {
		replayGuard = current.replay
	} else {
		guard, err := r.replayPolicy.NewGuard()
		if err != nil {
			r.link.message("could not cancel run: " + err.Error())
			return
		}
		replayGuard = guard
	}
	pending := pendingCancellation{
		request: target, openingCommandID: r.openingCommandForRun(target.RunID), policy: policy, replay: replayGuard,
	}
	if pending.openingCommandID == "" && r.execution.pendingCancel != nil && r.execution.pendingCancel.request.RunID == target.RunID {
		pending.openingCommandID = r.execution.pendingCancel.openingCommandID
	}
	if pending.openingCommandID == "" && r.workbench != nil {
		outbox := r.workbench.PendingRuns(r.session.current.ID)
		if len(outbox) > 0 && outbox[0].State == workbench.PendingRunCanceling &&
			outbox[0].CancelCommandID == target.CommandID {
			pending.openingCommandID = outbox[0].Command.CommandID
			pending.replay = outbox[0].CancelReplay
		}
	}
	r.execution.pendingCancel = &pending
	dispatcher := r.tasks.dispatch
	r.tasks.operations.Go(cancelRunOperation, true, func(ownerCtx context.Context, lease operationLease) {
		err := r.cancelRootRun(ownerCtx, target, pending.replay)
		_ = post(ownerCtx, dispatcher, func() {
			r.handleRuntimeCancellation(lease, pending, err)
		})
	})
}

// cancelRootRun makes cancellation idempotent at the terminal boundary. A run
// may finish between the user's gesture and the control request; in that case
// the authoritative snapshot that follows is the settlement of the same intent.
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
		// A rejected control request says nothing about the run itself. Keep the
		// conversation and cancellation target intact so the user can retry while
		// the runtime remains the source of truth for eventual settlement.
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
	if pending.policy == recoverProjectionFailure {
		r.prompt.SetBusy(false)
		r.syncAnimation()
		// The stream was rejected after Runtime had accepted the command. A
		// cancellation receipt cannot reconstruct durable Items that were never
		// projected, so the authoritative snapshot is the queue-admission fence.
		r.sessions.refreshInvalidatedSession(true)
		return
	}
	// The receipt names only the root Run. Its children and open tool calls
	// settle in the Runtime with their own outcomes, so the authoritative
	// snapshot, not the root's outcome, replaces the projection.
	r.status.note("canceled")
	r.prompt.SetBusy(false)
	r.syncAnimation()
	r.sessions.refreshInvalidatedSession(true)
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
			err = errors.Join(err, r.workbench.AcknowledgePendingResume(r.session.current.ID, pending.Command.CommandID))
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
	if _, retired := errors.AsType[*conversation.RetiredOpeningError](err); retired {
		return r.retireCanceledStart(pending)
	}
	opened, accepted := observedSegmentStream(opened, err)
	if !accepted {
		if !mutation.OutcomeUnknown(err) {
			return r.retireCanceledStart(pending)
		}
		return fmt.Errorf("reconcile run start during terminal close: %w", err)
	}
	validationErr := opened.ValidateStart()
	if identityErr := protocol.ValidateRunID(opened.RunID); identityErr != nil {
		return errors.Join(
			fmt.Errorf("reconcile run start during terminal close: accepted receipt: %w", identityErr),
			err,
			validationErr,
		)
	}
	cancelErr := r.cancelRuntimeNow(ctx, conversation.CancelRun{
		CommandID: pending.CancelCommandID,
		RunID:     opened.RunID,
		Reason:    unconfirmedStartCancellationReason,
	}, pending.CancelReplay)
	if cancelErr != nil {
		return errors.Join(err, validationErr, fmt.Errorf("cancel run opened during terminal close: %w", cancelErr))
	}
	return r.retireCanceledStart(pending)
}
