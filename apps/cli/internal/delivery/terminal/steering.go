package terminal

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	runworkflow "github.com/Tangerg/flame/apps/cli/internal/application/agent/run"
	"github.com/Tangerg/flame/apps/cli/internal/application/mutation"
	"github.com/Tangerg/flame/apps/cli/internal/application/workbench"
	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
)

type steerDesk struct {
	receipts     steerReceipts
	runtime      Runtime
	workbench    *workbench.Store
	replayPolicy mutation.ReplayPolicy
	composer     *promptComposer
	session      *sessionState
	runs         *runs
	tasks        taskRunner

	notify      func(string)
	report      func(workbenchConcern, error)
	drainQueue  func() bool
	readSession func(context.Context, string) (conversation.SessionSnapshot, error)
}

func (s *steerDesk) steerRequest(instruction string) (authoring.SteerRun, authoring.Message, error) {
	if instruction == "" {
		return authoring.SteerRun{}, authoring.Message{}, errors.New("/steer needs a non-empty instruction")
	}
	observed := s.runs.conversation()
	runID, segmentID := observed.RunID(), observed.SegmentID()
	if runID == "" || segmentID == "" || observed.Phase() != conversation.Running {
		return authoring.SteerRun{}, authoring.Message{}, errors.New("no observed run segment is available to steer")
	}
	draft, _, err := s.composer.draft()
	if err != nil {
		return authoring.SteerRun{}, authoring.Message{}, err
	}
	message := authoring.Message{Text: instruction, Attachments: slices.Clone(draft.Attachments)}
	if err := s.composer.validate(message); err != nil {
		return authoring.SteerRun{}, authoring.Message{}, err
	}
	request := authoring.SteerRun{CommandID: mutation.NewCommandID(), RunID: runID, SegmentID: segmentID, Message: message}
	if err := request.Validate(); err != nil {
		return authoring.SteerRun{}, authoring.Message{}, err
	}
	return request, draft, nil
}

func (s *steerDesk) steerRun(instruction string) error {
	instruction = strings.TrimSpace(instruction)
	request, draft, err := s.steerRequest(instruction)
	if err != nil {
		return err
	}
	sourceDraft := authoring.Message{Text: workbench.SteerSourcePrefix + instruction, Attachments: slices.Clone(draft.Attachments)}
	if err := s.composer.saveDraft(sourceDraft); err != nil {
		s.report(workbenchDraft, err)
		return fmt.Errorf("steer blocked: save command draft: %w", err)
	}
	s.report(workbenchDraft, nil)
	s.composer.restoreComposer(sourceDraft)
	s.composer.drafts.adopt(s.session.current.ID, sourceDraft)
	for _, slot := range []operationSlot{inputPreparationOperation, steerRunOperation} {
		if s.tasks.operations.Active(slot) {
			return slot.busy()
		}
	}
	started := s.tasks.sessionFence(inputPreparationOperation,
		func(ctx context.Context) (*workbench.PreparedInput, error) {
			blocks, err := s.runtime.PrepareInput(ctx, request.Message)
			if err != nil {
				return nil, err
			}
			return s.workbench.PrepareInput(ctx, request.Message, blocks)
		},
		func(prepared *workbench.PreparedInput, prepareErr error) {
			if prepareErr != nil {
				s.notify("prepare steer input: " + prepareErr.Error())
				s.drainQueue()
				return
			}
			current, _, err := s.composer.draft()
			if err != nil || !current.Equal(sourceDraft) {
				s.notify("steer preparation canceled because the draft changed")
				s.drainQueue()
				return
			}
			if err := s.deliverPreparedSteer(request, sourceDraft, prepared); err != nil {
				s.notify(err.Error())
				s.drainQueue()
			}
		},
	)
	if !started {
		return errors.New("another input preparation is already running")
	}
	return nil
}

func (s *steerDesk) deliverPreparedSteer(request authoring.SteerRun, sourceDraft authoring.Message, input *workbench.PreparedInput) error {
	pending, err := runworkflow.StageSteer(
		s.workbench, s.session.current.ID, request, sourceDraft, s.replayPolicy, input,
	)
	if err != nil {
		s.report(workbenchSteerOutbox, fmt.Errorf("save steer command journal: %w", err))
		return err
	}
	s.report(workbenchSteerOutbox, nil)
	s.composer.restoreComposer(authoring.Message{})
	s.composer.drafts.adopt(s.session.current.ID, authoring.Message{})
	s.receipts.track(pending)
	started := s.tasks.sessionSettlement(steerRunOperation,
		func(ctx context.Context) (runworkflow.SteerResult, error) {
			return runworkflow.DeliverSteer(
				ctx, s.runtime, s.workbench, pending, s.replayPolicy, runtimeRecoveryBackoff,
			)
		},
		func(result runworkflow.SteerResult, deliveryErr error) {
			s.settleSteer(result, deliveryErr)
		},
	)
	if !started {
		s.receipts.reject(pending)
		recovered, err := s.rejectSteer(pending)
		if err != nil {
			s.restoreSteerAttachments(request.Message.Attachments)
			return fmt.Errorf("%w; restore attachments: %w", steerRunOperation.busy(), err)
		}
		s.composer.restoreComposer(recovered)
		s.composer.drafts.adopt(s.session.current.ID, recovered)
		return steerRunOperation.busy()
	}
	return nil
}

func (s *steerDesk) settleSteer(result runworkflow.SteerResult, deliveryErr error) {
	switch result.Outcome {
	case mutation.Confirmed:
		s.receipts.accept(result)
		s.presentSteerReceipts()
		if err := s.acknowledgeSteer(result.Pending); err != nil {
			s.notify("steer accepted; local settlement pending: " + err.Error())
			return
		}
	case mutation.Rejected:
		s.receipts.reject(result.Pending)
		recovered, err := s.rejectSteer(result.Pending)
		if err != nil {
			s.restoreSteerAttachments(result.Pending.Message().Attachments)
			s.notify("steer run failed; restored attachments were not saved: " + err.Error())
			return
		}
		s.composer.restoreComposer(recovered)
		s.composer.drafts.adopt(s.session.current.ID, recovered)
		s.notify("steer run failed: " + deliveryErr.Error())
	case mutation.Unknown:
		s.notify("steer outcome is unknown; the original request is saved for recovery; inspect the session before issuing a new command: " + deliveryErr.Error())
	default:
		s.notify("steer settlement returned an invalid outcome")
	}
}

func (s *steerDesk) acknowledgeSteer(pending workbench.PendingSteer) error {
	current, _, err := s.composer.draft()
	if err != nil {
		return fmt.Errorf("read composer for steer settlement: %w", err)
	}
	if err := s.composer.saveDraft(current); err != nil {
		s.report(workbenchDraft, err)
		return fmt.Errorf("save current session draft: %w", err)
	}
	s.report(workbenchDraft, nil)
	if err := s.workbench.AcknowledgePendingSteer(s.session.current.ID, pending.CommandID()); err != nil {
		s.composer.history.Load(s.workbench.History())
		s.report(workbenchSteerOutbox, fmt.Errorf("settle accepted steer command: %w", err))
		return fmt.Errorf("retire accepted steer command: %w", err)
	}
	s.composer.history.Add(pending.Message())
	s.report(workbenchSteerOutbox, nil)
	s.report(workbenchHistory, nil)
	return nil
}

func (s *steerDesk) rejectSteer(pending workbench.PendingSteer) (authoring.Message, error) {
	current, _, err := s.composer.draft()
	if err != nil {
		return authoring.Message{}, fmt.Errorf("read composer for attachment recovery: %w", err)
	}
	if saveDraftErr := s.composer.saveDraft(current); saveDraftErr != nil {
		s.report(workbenchDraft, saveDraftErr)
		return authoring.Message{}, fmt.Errorf("save current session draft: %w", saveDraftErr)
	}
	s.report(workbenchDraft, nil)
	recovered, err := s.workbench.RejectPendingSteer(
		s.session.current.ID, pending.CommandID(), current,
	)
	if err != nil {
		s.report(workbenchSteerOutbox, fmt.Errorf("settle rejected steer command: %w", err))
		return authoring.Message{}, fmt.Errorf("save restored attachments: %w", err)
	}
	s.report(workbenchSteerOutbox, nil)
	return recovered, nil
}

func (s *steerDesk) restoreSteerAttachments(rejected []authoring.Attachment) {
	current, _, err := s.composer.draft()
	if err != nil {
		s.composer.insertAttachments(rejected)
		s.composer.scheduleDraftPersistence()
		return
	}
	s.composer.restoreComposer(workbench.MergeSteerAttachments(current, rejected))
}

func (s *steerDesk) refusedSteersNotice(refused []runworkflow.SteerRefusal) string {
	if len(refused) == 0 {
		return ""
	}
	causes := make([]string, 0, len(refused))
	for _, refusal := range refused {
		cause := refusal.Cause.Error()
		if refusal.SessionID != s.session.current.ID {
			cause = "session " + refusal.SessionID + ": " + cause
		}
		causes = append(causes, cause)
	}
	return "steer sent before restart was refused; attachments returned to the draft · " + strings.Join(causes, "; ")
}
