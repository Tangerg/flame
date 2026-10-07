package terminal

import (
	"context"
	"errors"
	"fmt"
	"time"

	runworkflow "github.com/Tangerg/flame/cli/internal/application/agent/run"
	"github.com/Tangerg/flame/cli/internal/application/mutation"
	"github.com/Tangerg/flame/cli/internal/application/retry"
	"github.com/Tangerg/flame/cli/internal/application/workbench"
	"github.com/Tangerg/flame/cli/internal/domain/authoring/prompt"
	"github.com/Tangerg/flame/cli/internal/domain/authoring/replay"
	"github.com/Tangerg/flame/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/runtime/protocol"
)

const (
	runtimeControlTimeout              = 5 * time.Second
	unconfirmedStartCancellationReason = "canceled while start delivery was unconfirmed"
)

func (a *app) cancel() {
	if a.dialogs.approval != nil {
		a.answerApproval(approvalDenyOnce)
		return
	}
	if a.dialogs.questionnaire != nil {
		a.finishQuestionnaire(true)
		return
	}
	if a.operations.Active(inputPreparationOperation) {
		a.operations.Cancel(inputPreparationOperation)
		if entry, reserved := a.queue.Dispatching(a.session.current.ID); reserved {
			pending, found := pendingRunByCommandID(a.workbench.PendingRuns(a.session.current.ID), entry.CommandID)
			if found && pending.State == workbench.PendingRunQueued {
				a.queue.ReleaseDispatch(a.session.current.ID)
			}
		}
		a.syncQueue()
		a.prompt.SetBusy(a.runAdmissionBlocked())
		a.message("input preparation canceled; the prompt remains editable")
		return
	}
	if a.execution.pendingCancel != nil {
		a.status.doing = "retrying cancellation"
		a.requestRuntimeCancellation(a.execution.pendingCancel.request, a.execution.pendingCancel.policy)
		return
	}
	if !a.execution.conversation.Busy() && !a.execution.following {
		return
	}
	a.status.doing = "canceling"
	runID := a.execution.conversation.RunID()
	if runID == "" {
		pending, staged, err := a.stageOpeningCancellation()
		if err != nil {
			a.message("could not preserve cancellation of unconfirmed run: " + err.Error())
			return
		}
		if !staged {
			return
		}
		a.dropStream()
		if err := a.execution.conversation.CancelStarting(); err != nil {
			a.fail(err)
			return
		}
		a.prompt.SetBusy(false)
		a.status.note("canceled · reconciling runtime delivery")
		a.syncAnimation()
		a.reconcileCanceledStart(pending)
		return
	}
	a.cancelRuntime(conversation.CancelRun{RunID: runID, Reason: "canceled by the terminal user"})
}

func (a *app) stageOpeningCancellation() (workbench.PendingRun, bool, error) {
	entry, ok := a.queue.Dispatching(a.session.current.ID)
	if !ok {
		return workbench.PendingRun{}, false, nil
	}
	if entry.CommandID == "" {
		return workbench.PendingRun{}, false, errors.New("dispatching queue entry is no longer available")
	}
	if pending, found := pendingRunByCommandID(a.workbench.PendingRuns(a.session.current.ID), entry.CommandID); found &&
		pending.State == workbench.PendingRunQueued {
		// Input preparation has not published a dispatch intent. Closing the
		// operation owner will discard its result without a Runtime mutation.
		return workbench.PendingRun{}, false, nil
	}
	cancelReplay, err := a.replayPolicy.NewGuard()
	if err != nil {
		return workbench.PendingRun{}, false, err
	}
	if _, err := a.workbench.MarkPendingRunCanceling(a.session.current.ID, entry.CommandID, cancelReplay); err != nil {
		return workbench.PendingRun{}, false, err
	}
	pending, ok := pendingRunByCommandID(a.workbench.PendingRuns(a.session.current.ID), entry.CommandID)
	if !ok {
		return workbench.PendingRun{}, false, errors.New("canceling run start disappeared from the durable outbox")
	}
	return pending, true, nil
}

func (a *app) reconcileCanceledStart(pending workbench.PendingRun) {
	dispatcher := a.loop.Dispatcher()
	a.operations.GoSession(pendingRunRecoveryOperation, false, func(ctx context.Context, lease operationLease) {
		opened, err := openStartRunWithBackoff(
			ctx, a.runtime, pending.Command, pending.Replay, a.replayPolicy, runtimeRecoveryBackoff,
		)
		if context.Cause(ctx) != nil {
			return
		}
		_ = post(ctx, dispatcher, func() {
			if !a.operations.Current(lease) || a.closed || a.session.current.ID != pending.Command.SessionID ||
				!a.operations.Release(lease) {
				return
			}
			observed, accepted := observedSegmentStream(opened, err)
			if !accepted {
				if mutation.OutcomeUnknown(err) {
					a.fail(fmt.Errorf("reconcile canceled start: %w", err))
					return
				}
				if retireErr := a.retireCanceledStart(pending); retireErr != nil {
					a.fail(errors.Join(err, retireErr))
				}
				return
			}
			validationErr := observed.ValidateStart()
			if identityErr := protocol.ValidateRunID(observed.RunID); identityErr != nil {
				a.fail(errors.Join(
					fmt.Errorf("reconcile canceled start: accepted receipt: %w", identityErr),
					err,
					validationErr,
				))
				return
			}
			if receiptErr := errors.Join(err, validationErr); receiptErr != nil {
				a.message("runtime returned an invalid start receipt; canceling accepted run: " + receiptErr.Error())
			}
			a.requestRuntimeCancellation(conversation.CancelRun{
				CommandID: pending.CancelCommandID,
				RunID:     observed.RunID,
				Reason:    unconfirmedStartCancellationReason,
			}, recoverCanceledOpening)
		})
	})
}

func openStartRunWithBackoff(
	ctx context.Context,
	runtime runworkflow.Lifecycle,
	command prompt.StartRun,
	replayGuard replay.Guard,
	replayPolicy mutation.ReplayPolicy,
	backoff retry.Backoff,
) (conversation.SegmentStream, error) {
	return mutation.ConfirmAdmitted(
		ctx, backoff, mutation.ReplayAdmission(replayPolicy, replayGuard),
		func(ctx context.Context) (conversation.SegmentStream, error) {
			return runtime.StartRun(ctx, command)
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

func (a *app) retireCanceledStart(pending workbench.PendingRun) error {
	if err := a.retireQueuedCommand(pending.Command.SessionID, pending.Command.CommandID); err != nil {
		return fmt.Errorf("retire canceled start: %w", err)
	}
	a.status.note("canceled")
	a.drainQueue()
	return nil
}

func (a *app) activeCancellation() (conversation.CancelRun, bool) {
	if a.execution.projectionFailed {
		return conversation.CancelRun{}, false
	}
	if runID := a.execution.conversation.RunID(); runID != "" && a.execution.conversation.Busy() {
		return conversation.CancelRun{RunID: runID, Reason: "terminal closed"}, true
	}
	if a.execution.openingRunID != "" && a.execution.conversation.Busy() {
		return conversation.CancelRun{RunID: a.execution.openingRunID, Reason: "terminal closed"}, true
	}
	return conversation.CancelRun{}, false
}

func (a *app) cancelRuntime(target conversation.CancelRun) {
	a.requestRuntimeCancellation(target, applyRuntimeSettlement)
}

// cancelRuntimePreservingFailure stops a run whose event stream has already been
// rejected locally. The cancellation receipt proves cleanup, but only a cold
// Session read can replace the now-untrustworthy transcript projection.
func (a *app) cancelRuntimePreservingFailure(target conversation.CancelRun) {
	a.requestRuntimeCancellation(target, recoverProjectionFailure)
}

type cancellationResultPolicy uint8

const (
	applyRuntimeSettlement cancellationResultPolicy = iota
	recoverProjectionFailure
	recoverCanceledOpening
)

type pendingCancellation struct {
	request          conversation.CancelRun
	openingCommandID replay.CommandID
	policy           cancellationResultPolicy
	replay           replay.Guard
}

func (a *app) requestRuntimeCancellation(target conversation.CancelRun, policy cancellationResultPolicy) {
	if target.CommandID == "" {
		commandID := mutation.NewCommandID()
		target.CommandID = commandID
	}
	var replayGuard replay.Guard
	if current := a.execution.pendingCancel; current != nil && current.request.CommandID == target.CommandID {
		replayGuard = current.replay
	} else {
		guard, err := a.replayPolicy.NewGuard()
		if err != nil {
			a.message("could not cancel run: " + err.Error())
			return
		}
		replayGuard = guard
	}
	pending := pendingCancellation{
		request: target, openingCommandID: a.openingCommandForRun(target.RunID), policy: policy, replay: replayGuard,
	}
	if pending.openingCommandID == "" && a.execution.pendingCancel != nil && a.execution.pendingCancel.request.RunID == target.RunID {
		pending.openingCommandID = a.execution.pendingCancel.openingCommandID
	}
	if pending.openingCommandID == "" && a.workbench != nil {
		outbox := a.workbench.PendingRuns(a.session.current.ID)
		if len(outbox) > 0 && outbox[0].State == workbench.PendingRunCanceling &&
			outbox[0].CancelCommandID == target.CommandID {
			pending.openingCommandID = outbox[0].Command.CommandID
			pending.replay = outbox[0].CancelReplay
		}
	}
	a.execution.pendingCancel = &pending
	dispatcher := a.loop.Dispatcher()
	a.operations.Go(cancelRunOperation, true, func(ownerCtx context.Context, lease operationLease) {
		err := a.cancelRootRun(ownerCtx, target, pending.replay)
		_ = post(ownerCtx, dispatcher, func() {
			a.handleRuntimeCancellation(lease, pending, err)
		})
	})
}

// cancelRootRun makes cancellation idempotent at the terminal boundary. A run
// may finish between the user's gesture and the control request; in that case
// the authoritative snapshot that follows is the settlement of the same intent.
func (a *app) cancelRootRun(
	ctx context.Context,
	target conversation.CancelRun,
	replayGuard replay.Guard,
) error {
	result, err := mutation.ConfirmAdmitted(
		ctx, runtimeRecoveryBackoff, mutation.ReplayAdmission(a.replayPolicy, replayGuard),
		func(ctx context.Context) (conversation.RunCancellation, error) {
			attemptCtx, cancel := context.WithTimeout(ctx, runtimeControlTimeout)
			defer cancel()
			return a.runtime.CancelRun(attemptCtx, target)
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

func (a *app) handleRuntimeCancellation(
	lease operationLease,
	pending pendingCancellation,
	err error,
) {
	if !a.operations.Current(lease) || a.closed {
		return
	}
	if err != nil {
		// A rejected control request says nothing about the run itself. Keep the
		// conversation and cancellation target intact so the user can retry while
		// the runtime remains the source of truth for eventual settlement.
		a.message("could not cancel run: " + err.Error())
		return
	}
	if retireErr := a.retireCanceledRuntimeOwnership(pending.request.RunID, pending.openingCommandID); retireErr != nil {
		failure := fmt.Errorf("retire canceled runtime ownership: %w", retireErr)
		a.reportWorkbenchIssue(workbenchCancellationOwnership, failure)
		a.message("could not " + failure.Error())
		a.retryCanceledRuntimeOwnership(pending.request.RunID, pending.openingCommandID)
	} else {
		a.reportWorkbenchIssue(workbenchCancellationOwnership, nil)
	}
	if current := a.execution.pendingCancel; current != nil && current.request.CommandID == pending.request.CommandID {
		a.execution.pendingCancel = nil
	}
	a.execution.openingRunID = ""
	a.dropStream()
	if pending.policy == recoverProjectionFailure {
		a.prompt.SetBusy(false)
		a.syncAnimation()
		// The stream was rejected after Runtime had accepted the command. A
		// cancellation receipt cannot reconstruct durable Items that were never
		// projected, so the authoritative snapshot is the queue-admission fence.
		a.session.invalidated = true
		a.refreshInvalidatedSession(true)
		return
	}
	// The receipt names only the root Run. Its children and open tool calls
	// settle in the Runtime with their own outcomes, so the authoritative
	// snapshot, not the root's outcome, replaces the projection.
	a.status.note("canceled")
	a.prompt.SetBusy(false)
	a.syncAnimation()
	a.session.invalidated = true
	a.refreshInvalidatedSession(true)
}

func (a *app) openingCommandForRun(runID string) replay.CommandID {
	if runID == "" || runID != a.execution.openingRunID {
		return ""
	}
	entry, ok := a.queue.Dispatching(a.session.current.ID)
	if !ok {
		return ""
	}
	return entry.CommandID
}

func (a *app) retireCanceledRuntimeOwnership(runID string, openingCommandID replay.CommandID) error {
	var err error
	if a.workbench != nil {
		if pending, ok := a.workbench.PendingResume(a.session.current.ID); ok && pending.Command.RunID == runID {
			err = errors.Join(err, a.workbench.AcknowledgePendingResume(a.session.current.ID, pending.Command.CommandID))
		}
	}
	if openingCommandID != "" {
		err = errors.Join(err, a.retireQueuedCommand(a.session.current.ID, openingCommandID))
	}
	return err
}

func (a *app) cancelRuntimeNow(
	ownerCtx context.Context,
	target conversation.CancelRun,
	replayGuard replay.Guard,
) error {
	if target.CommandID == "" {
		commandID := mutation.NewCommandID()
		target.CommandID = commandID
		guard, err := a.replayPolicy.NewGuard()
		if err != nil {
			return err
		}
		replayGuard = guard
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ownerCtx), runtimeControlTimeout)
	defer cancel()
	result, err := mutation.ConfirmAdmitted(
		ctx, runtimeRecoveryBackoff, mutation.ReplayAdmission(a.replayPolicy, replayGuard),
		func(ctx context.Context) (conversation.RunCancellation, error) {
			return a.runtime.CancelRun(ctx, target)
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

func (a *app) cancelOpeningRunNow(ownerCtx context.Context, pending workbench.PendingRun) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ownerCtx), runtimeControlTimeout)
	defer cancel()
	opened, err := openStartRunWithBackoff(
		ctx, a.runtime, pending.Command, pending.Replay, a.replayPolicy, runtimeRecoveryBackoff,
	)
	opened, accepted := observedSegmentStream(opened, err)
	if !accepted {
		if !mutation.OutcomeUnknown(err) {
			return a.retireCanceledStart(pending)
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
	cancelErr := a.cancelRuntimeNow(ctx, conversation.CancelRun{
		CommandID: pending.CancelCommandID,
		RunID:     opened.RunID,
		Reason:    unconfirmedStartCancellationReason,
	}, pending.CancelReplay)
	if cancelErr != nil {
		return errors.Join(err, validationErr, fmt.Errorf("cancel run opened during terminal close: %w", cancelErr))
	}
	return a.retireCanceledStart(pending)
}
