package terminal

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/Tangerg/flame/cli/internal/application/agent/mutation"
	runworkflow "github.com/Tangerg/flame/cli/internal/application/agent/run"
	"github.com/Tangerg/flame/cli/internal/application/agent/workbench"
	"github.com/Tangerg/flame/cli/internal/domain/agent"
)

func (a *app) steerRun(instruction string) error {
	instruction = strings.TrimSpace(instruction)
	if instruction == "" {
		return errors.New("/steer needs a non-empty instruction")
	}
	runID, segmentID := a.execution.conversation.RunID(), a.execution.conversation.SegmentID()
	if runID == "" || segmentID == "" || a.execution.conversation.Phase() != agent.ConversationRunning {
		return errors.New("no observed run segment is available to steer")
	}
	draft, _, err := a.currentDraft()
	if err != nil {
		return err
	}
	message := agent.Message{Text: instruction, Attachments: slices.Clone(draft.Attachments)}
	if validateMessageCapabilitiesErr := a.validateMessageCapabilities(message); validateMessageCapabilitiesErr != nil {
		return validateMessageCapabilitiesErr
	}
	commandID := mutation.NewCommandID()
	request := agent.SteerRun{CommandID: commandID, RunID: runID, SegmentID: segmentID, Message: message}
	if validateErr := request.Validate(); validateErr != nil {
		return validateErr
	}

	// Reconstruct the parsed command as a durable ownership precondition. If the
	// process stops before staging, restart restores the command for retry. The
	// following aggregate replacement then transfers it and its attachments into
	// the steer journal atomically.
	sourceDraft := agent.Message{Text: "/steer " + instruction, Attachments: slices.Clone(draft.Attachments)}
	if saveDraftErr := a.saveDraft(sourceDraft); saveDraftErr != nil {
		a.reportWorkbenchIssue(workbenchDraft, saveDraftErr)
		return fmt.Errorf("steer blocked: save command draft: %w", saveDraftErr)
	}
	a.reportWorkbenchIssue(workbenchDraft, nil)
	a.restoreComposer(sourceDraft)
	a.draftState.Reset(a.session.current.ID, sourceDraft)
	if a.operations.Active(inputPreparationOperation) || a.operations.Active(steerRunOperation) {
		return errors.New("another input or steer operation is already running")
	}
	started := a.runSessionAdmissionFence(inputPreparationOperation, false,
		func(ctx context.Context) (*workbench.PreparedInput, error) {
			blocks, err := a.runtime.PrepareInput(ctx, request.Message)
			if err != nil {
				return nil, err
			}
			return a.workbench.PrepareInput(ctx, request.Message, blocks)
		},
		func(prepared *workbench.PreparedInput, prepareErr error) {
			if prepareErr != nil {
				a.message("prepare steer input: " + prepareErr.Error())
				a.drainQueue()
				return
			}
			current, _, err := a.currentDraft()
			if err != nil || !current.Equal(sourceDraft) {
				a.message("steer preparation canceled because the draft changed")
				a.drainQueue()
				return
			}
			if err := a.deliverPreparedSteer(request, sourceDraft, prepared); err != nil {
				a.message(err.Error())
				a.drainQueue()
			}
		},
	)
	if !started {
		return errors.New("another input preparation is already running")
	}
	return nil
}

func (a *app) deliverPreparedSteer(request agent.SteerRun, sourceDraft agent.Message, input *workbench.PreparedInput) error {
	pending, err := runworkflow.StageSteer(
		a.workbench, a.session.current.ID, request, sourceDraft, commandReplayPolicy(a.runtimeProfile), input,
	)
	if err != nil {
		a.reportWorkbenchIssue(workbenchSteerOutbox, fmt.Errorf("save steer command journal: %w", err))
		return err
	}
	a.reportWorkbenchIssue(workbenchSteerOutbox, nil)
	a.restoreComposer(agent.Message{})
	a.draftState.Reset(a.session.current.ID, agent.Message{})
	a.steers.track(pending)
	started := a.runSessionSettlement(steerRunOperation, false,
		func(ctx context.Context) (runworkflow.SteerResult, error) {
			return runworkflow.DeliverSteer(
				ctx, a.runtime, pending, commandReplayPolicy(a.runtimeProfile), runtimeRecoveryBackoff,
			)
		},
		func(result runworkflow.SteerResult, deliveryErr error) {
			a.settleSteer(result, deliveryErr)
		},
	)
	if !started {
		a.steers.reject(pending)
		recovered, err := a.rejectSteer(pending)
		if err != nil {
			a.restoreComposer(workbenchMergeSteerAttachments(a, request.Message.Attachments))
			return fmt.Errorf("another steer operation is already running; restore attachments: %w", err)
		}
		a.restoreComposer(recovered)
		a.draftState.Reset(a.session.current.ID, recovered)
		return errors.New("another steer operation is already running")
	}
	return nil
}

func (a *app) settleSteer(result runworkflow.SteerResult, deliveryErr error) {
	switch result.Outcome {
	case mutation.Confirmed:
		a.steers.accept(result)
		a.presentSteerReceipts()
		if err := a.acknowledgeSteer(result.Pending); err != nil {
			a.message("steer accepted; local settlement pending: " + err.Error())
			return
		}
	case mutation.Rejected:
		a.steers.reject(result.Pending)
		recovered, err := a.rejectSteer(result.Pending)
		if err != nil {
			a.restoreComposer(workbenchMergeSteerAttachments(a, result.Pending.Message().Attachments))
			a.message("steer run failed; restored attachments were not saved: " + err.Error())
			return
		}
		a.restoreComposer(recovered)
		a.draftState.Reset(a.session.current.ID, recovered)
		a.message("steer run failed: " + deliveryErr.Error())
	case mutation.Unknown:
		a.message("steer outcome is unknown; the original request is saved for recovery; inspect the session before issuing a new command: " + deliveryErr.Error())
	default:
		a.message("steer settlement returned an invalid outcome")
	}
}

func (a *app) acknowledgeSteer(pending workbench.PendingSteer) error {
	current, _, err := a.currentDraft()
	if err != nil {
		return fmt.Errorf("read composer for steer settlement: %w", err)
	}
	if err := a.saveDraft(current); err != nil {
		a.reportWorkbenchIssue(workbenchDraft, err)
		return fmt.Errorf("save current session draft: %w", err)
	}
	a.reportWorkbenchIssue(workbenchDraft, nil)
	if err := a.workbench.AcknowledgePendingSteer(a.session.current.ID, pending.CommandID()); err != nil {
		a.history.Load(a.workbench.History())
		a.reportWorkbenchIssue(workbenchSteerOutbox, fmt.Errorf("settle accepted steer command: %w", err))
		return fmt.Errorf("retire accepted steer command: %w", err)
	}
	a.history.Add(pending.Message())
	a.reportWorkbenchIssue(workbenchSteerOutbox, nil)
	a.reportWorkbenchIssue(workbenchHistory, nil)
	return nil
}

func (a *app) rejectSteer(pending workbench.PendingSteer) (agent.Message, error) {
	current, _, err := a.currentDraft()
	if err != nil {
		return agent.Message{}, fmt.Errorf("read composer for attachment recovery: %w", err)
	}
	if saveDraftErr := a.saveDraft(current); saveDraftErr != nil {
		a.reportWorkbenchIssue(workbenchDraft, saveDraftErr)
		return agent.Message{}, fmt.Errorf("save current session draft: %w", saveDraftErr)
	}
	a.reportWorkbenchIssue(workbenchDraft, nil)
	recovered, err := a.workbench.RejectPendingSteer(
		a.session.current.ID, pending.CommandID(), current,
	)
	if err != nil {
		a.reportWorkbenchIssue(workbenchSteerOutbox, fmt.Errorf("settle rejected steer command: %w", err))
		return agent.Message{}, fmt.Errorf("save restored attachments: %w", err)
	}
	a.reportWorkbenchIssue(workbenchSteerOutbox, nil)
	return recovered, nil
}

func workbenchMergeSteerAttachments(a *app, rejected []agent.Attachment) agent.Message {
	current, _, err := a.currentDraft()
	if err != nil {
		return agent.Message{Attachments: slices.Clone(rejected)}
	}
	return workbench.MergeSteerAttachments(current, rejected)
}
