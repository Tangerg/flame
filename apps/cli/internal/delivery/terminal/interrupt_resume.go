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
func (r *runs) resumeInterrupts(review *interruptReview) {
	answers, err := review.Responses()
	if err != nil {
		r.fail(fmt.Errorf("commit interrupt review: %w", err))
		return
	}
	runID := r.execution.conversation.RunID()
	if err := r.execution.conversation.ValidateInterruptReview(runID, review.Items()); err != nil {
		failure := fmt.Errorf("resume blocked: %w", err)
		review.ReportSubmissionFailure(failure)
		r.fail(failure)
		return
	}
	commandID := mutation.NewCommandID()
	command := conversation.ResumeRun{CommandID: commandID, RunID: runID, Answers: answers}
	replayGuard, err := r.replayPolicy.NewGuard()
	if err != nil {
		failure := fmt.Errorf("resume blocked: %w", err)
		review.ReportSubmissionFailure(failure)
		r.fail(failure)
		return
	}
	if r.workbench != nil {
		pending := workbench.PendingResume{
			Command: command.Clone(), Interrupts: review.Items(), Replay: replayGuard,
		}
		if err := r.workbench.StagePendingResume(r.session.current.ID, pending, nil); err != nil {
			failure := fmt.Errorf("resume blocked: save interrupt decisions: %w", err)
			review.ReportSubmissionFailure(failure)
			r.link.message(failure.Error())
			r.status.note("resume blocked · review preserved")
			if reopenErr := r.prompts.reopenCompletedInterruptReview(review); reopenErr != nil {
				r.fail(errors.Join(failure, reopenErr))
			}
			return
		}
	}
	r.deliverInterruptResume(review, command)
}

func (r *runs) deliverInterruptResume(
	review *interruptReview,
	command conversation.ResumeRun,
) {
	pending, ok := r.workbench.PendingResume(r.session.current.ID)
	if !ok || pending.Command.CommandID != command.CommandID {
		r.fail(errors.New("submitted resume is absent from the durable outbox"))
		return
	}
	r.status.active("resuming")
	r.syncAnimation()
	r.followOpening(func(ctx context.Context) (conversation.SegmentStream, error) {
		stream, err := runworkflow.OpenPendingResume(ctx, r.runtime, r.workbench, pending, r.replayPolicy)
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
			r.prompts.release()
			r.settleAcknowledgedResume(command.CommandID)
			if r.execution.conversation.RunID() != opened.RunID {
				r.followRecoveredSession()
				return rejectOpenedStream
			}
			return followOpenedStream
		},
		rejected: func(failure error) error {
			if _, retired := errors.AsType[*conversation.RetiredOpeningError](failure); retired {
				r.prompts.release()
				r.settleAcknowledgedResume(command.CommandID)
				return failure
			}
			if _, accepted := conversation.AcceptedMutationReceipt(failure); accepted {
				r.prompts.release()
				r.cancelRuntimePreservingFailure(conversation.CancelRun{
					RunID: command.RunID, Reason: "runtime returned an invalid resume receipt",
				})
				return failure
			}
			return r.restoreRejectedInterruptReview(review, command, failure)
		},
	})
}

func (r *runs) settleAcknowledgedResume(commandID authoring.CommandID) {
	if err := r.retireAcknowledgedResume(commandID); err != nil {
		r.report(workbenchResumeOutbox, err)
		r.link.message("could not settle acknowledged interrupt decisions: " + err.Error())
		r.retryAuthoringSettlement(
			resumeSettlementOperation,
			func() error { return r.retireAcknowledgedResume(commandID) },
			func() { r.report(workbenchResumeOutbox, nil) },
		)
		return
	}
	r.report(workbenchResumeOutbox, nil)
}

func (r *runs) retireAcknowledgedResume(commandID authoring.CommandID) error {
	if _, ok := r.workbench.PendingResume(r.session.current.ID); !ok {
		return nil
	}
	// The store claims the identity under its own lock, so re-deciding it here
	// would both restate the rule and read it a moment before the claim.
	return r.workbench.AcknowledgePendingResume(r.session.current.ID, commandID)
}

func (r *runs) restoreRejectedInterruptReview(review *interruptReview, command conversation.ResumeRun, failure error) error {
	callFailure, refused := errors.AsType[*resumeRunCallError](failure)
	if !refused || mutation.OutcomeUnknown(callFailure.err) || r.prompts.review != review ||
		r.execution.conversation.Phase() != conversation.Waiting || r.execution.conversation.RunID() != command.RunID {
		return failure
	}
	if r.workbench != nil {
		if err := r.workbench.RejectPendingResume(r.session.current.ID, command.CommandID); err != nil {
			return errors.Join(failure, fmt.Errorf("release refused interrupt decisions: %w", err))
		}
	}
	r.execution.following = false
	if r.session.invalidated {
		r.status.note("resume refused · refreshing session")
		r.link.refreshInvalidatedSession(false)
		return nil
	}
	r.status.note("resume refused · review preserved")
	review.ReportSubmissionFailure(fmt.Errorf("resume refused: %w", callFailure.err))
	if err := r.prompts.reopenCompletedInterruptReview(review); err != nil {
		return errors.Join(failure, err)
	}
	return nil
}

// abortRun cancels the waiting Run whose interrupts the user abandoned.
func (r *runs) abortRun(reason string) {
	if runID := r.execution.conversation.RunID(); runID != "" {
		r.cancelRuntime(conversation.CancelRun{RunID: runID, Reason: reason})
	}
}

// openInterrupts presents the interrupts a waiting Run raised.
func (r *runs) openInterrupts(interrupts []conversation.Interrupt) {
	if r.prompts.open(interrupts) {
		r.link.raiseAttention(interruptAttention(interrupts))
	}
}
