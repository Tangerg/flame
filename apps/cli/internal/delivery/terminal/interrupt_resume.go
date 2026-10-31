package terminal

import (
	"context"
	"errors"
	"fmt"

	runworkflow "github.com/Tangerg/flame/apps/cli/internal/application/agent/run"
	"github.com/Tangerg/flame/apps/cli/internal/application/mutation"
	"github.com/Tangerg/flame/apps/cli/internal/application/workbench"
	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
)

// resumeInterrupts stages and delivers a completed review.
func (a *app) resumeInterrupts(review *interruptReview) {
	answers, err := review.Responses()
	if err != nil {
		a.fail(fmt.Errorf("commit interrupt review: %w", err))
		return
	}
	runID := a.execution.conversation.RunID()
	if err := a.execution.conversation.ValidateInterruptReview(runID, review.Items()); err != nil {
		failure := fmt.Errorf("resume blocked: %w", err)
		review.ReportSubmissionFailure(failure)
		a.fail(failure)
		return
	}
	commandID := mutation.NewCommandID()
	command := conversation.ResumeRun{CommandID: commandID, RunID: runID, Answers: answers}
	replayGuard, err := a.replayPolicy.NewGuard()
	if err != nil {
		failure := fmt.Errorf("resume blocked: %w", err)
		review.ReportSubmissionFailure(failure)
		a.fail(failure)
		return
	}
	if a.workbench != nil {
		pending := workbench.PendingResume{
			Command: command.Clone(), Interrupts: review.Items(), Replay: replayGuard,
		}
		if err := a.workbench.StagePendingResume(a.session.current.ID, pending, nil); err != nil {
			failure := fmt.Errorf("resume blocked: save interrupt decisions: %w", err)
			review.ReportSubmissionFailure(failure)
			a.message(failure.Error())
			a.status.note("resume blocked · review preserved")
			if reopenErr := a.prompts.reopenCompletedInterruptReview(review); reopenErr != nil {
				a.fail(errors.Join(failure, reopenErr))
			}
			return
		}
	}
	a.deliverInterruptResume(review, command)
}

func (a *app) deliverInterruptResume(
	review *interruptReview,
	command conversation.ResumeRun,
) {
	pending, ok := a.workbench.PendingResume(a.session.current.ID)
	if !ok || pending.Command.CommandID != command.CommandID {
		a.fail(errors.New("submitted resume is absent from the durable outbox"))
		return
	}
	a.status.active("resuming")
	a.syncAnimation()
	a.followOpening(func(ctx context.Context) (conversation.SegmentStream, error) {
		stream, err := runworkflow.OpenPendingResume(ctx, a.runtime, a.workbench, pending, a.replayPolicy)
		if err != nil {
			if _, accepted := conversation.AcceptedMutationReceipt(err); accepted {
				return conversation.SegmentStream{}, err
			}
			return conversation.SegmentStream{}, &resumeRunCallError{err: err}
		}
		if err := stream.ValidateResume(command.RunID, command.Message); err != nil {
			return conversation.SegmentStream{}, conversation.NewAcceptedMutationError(stream, fmt.Errorf("resume run: %w", err))
		}
		return stream, nil
	}, streamOpeningObserver{
		persistent: true,
		accepted: func(opened conversation.SegmentStream) streamOpeningDisposition {
			a.prompts.release()
			a.settleAcknowledgedResume(command.CommandID)
			if a.execution.conversation.RunID() != opened.RunID {
				a.followRecoveredSession()
				return rejectOpenedStream
			}
			return followOpenedStream
		},
		rejected: func(failure error) error {
			if _, retired := errors.AsType[*conversation.RetiredOpeningError](failure); retired {
				a.prompts.release()
				a.settleAcknowledgedResume(command.CommandID)
				return failure
			}
			if _, accepted := conversation.AcceptedMutationReceipt(failure); accepted {
				a.prompts.release()
				a.cancelRuntimePreservingFailure(conversation.CancelRun{
					RunID: command.RunID, Reason: "runtime returned an invalid resume receipt",
				})
				return failure
			}
			return a.restoreRejectedInterruptReview(review, command, failure)
		},
	})
}

func (a *app) settleAcknowledgedResume(commandID authoring.CommandID) {
	if err := a.retireAcknowledgedResume(commandID); err != nil {
		a.reportWorkbenchIssue(workbenchResumeOutbox, err)
		a.message("could not settle acknowledged interrupt decisions: " + err.Error())
		a.retryAuthoringSettlement(
			resumeSettlementOperation,
			func() error { return a.retireAcknowledgedResume(commandID) },
			func() { a.reportWorkbenchIssue(workbenchResumeOutbox, nil) },
		)
		return
	}
	a.reportWorkbenchIssue(workbenchResumeOutbox, nil)
}

func (a *app) retireAcknowledgedResume(commandID authoring.CommandID) error {
	if _, ok := a.workbench.PendingResume(a.session.current.ID); !ok {
		return nil
	}
	// The store claims the identity under its own lock, so re-deciding it here
	// would both restate the rule and read it a moment before the claim.
	return a.workbench.AcknowledgePendingResume(a.session.current.ID, commandID)
}

func (a *app) restoreRejectedInterruptReview(review *interruptReview, command conversation.ResumeRun, failure error) error {
	callFailure, refused := errors.AsType[*resumeRunCallError](failure)
	if !refused || mutation.OutcomeUnknown(callFailure.err) || a.prompts.review != review ||
		a.execution.conversation.Phase() != conversation.Waiting || a.execution.conversation.RunID() != command.RunID {
		return failure
	}
	if a.workbench != nil {
		if err := a.workbench.RejectPendingResume(a.session.current.ID, command.CommandID); err != nil {
			return errors.Join(failure, fmt.Errorf("release refused interrupt decisions: %w", err))
		}
	}
	a.execution.following = false
	if a.session.invalidated {
		a.status.note("resume refused · refreshing session")
		a.refreshInvalidatedSession(false)
		return nil
	}
	a.status.note("resume refused · review preserved")
	review.ReportSubmissionFailure(fmt.Errorf("resume refused: %w", callFailure.err))
	if err := a.prompts.reopenCompletedInterruptReview(review); err != nil {
		return errors.Join(failure, err)
	}
	return nil
}

// abortRun cancels the waiting Run whose interrupts the user abandoned.
func (a *app) abortRun(reason string) {
	if runID := a.execution.conversation.RunID(); runID != "" {
		a.cancelRuntime(conversation.CancelRun{RunID: runID, Reason: reason})
	}
}

// openInterrupts presents the interrupts a waiting Run raised.
func (a *app) openInterrupts(interrupts []conversation.Interrupt) {
	if a.prompts.open(interrupts) {
		a.raiseAttention(interruptAttention(interrupts))
	}
}
