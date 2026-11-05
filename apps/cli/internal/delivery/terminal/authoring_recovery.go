package terminal

import (
	"context"
	"errors"
	"fmt"

	runworkflow "github.com/Tangerg/flame/apps/cli/internal/application/agent/run"
	"github.com/Tangerg/flame/apps/cli/internal/application/mutation"
	"github.com/Tangerg/flame/apps/cli/internal/application/workbench"
	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
)

// restoreSessionOutbox resumes durable runtime deliveries owned by the active
// session. It belongs to projection installation rather than process startup:
// every session can have an independent outbox, including one first visited
// after this process began.
func (r *runs) restoreSessionOutbox() {
	r.restorePendingResume()
	r.restorePendingRuns()
}

func (r *runs) restorePendingRuns() {
	pending := r.workbench.PendingRuns(r.session.current.ID)
	if len(pending) == 0 {
		return
	}
	if pending[0].State != workbench.PendingRunQueued && !r.replayPolicy.SameStore(pending[0].Replay) {
		r.fail(errors.New("recover pending run: command belongs to another runtime"))
		return
	}
	if err := r.restorePendingQueue(); err != nil {
		r.fail(err)
		return
	}
	if pending[0].State == workbench.PendingRunCanceling {
		r.reconcileCanceledStart(pending[0])
		return
	}
	if pending[0].State == workbench.PendingRunDispatching {
		if r.execution.conversation.Busy() {
			r.reconcilePendingRun(pending[0])
			return
		}
		r.replayPendingRun(pending[0])
		return
	}
	if !r.execution.conversation.Busy() {
		r.drainQueue()
	}
}

func (r *runs) restorePendingResume() {
	pending, ok := r.workbench.PendingResume(r.session.current.ID)
	if !ok {
		return
	}
	if !r.replayPolicy.SameStore(pending.Replay) {
		r.fail(errors.New("recover interrupt decisions: command belongs to another runtime"))
		return
	}
	if err := r.execution.conversation.ValidateInterruptReview(pending.Command.RunID, pending.Interrupts); err != nil {
		r.reconcilePendingResume(pending)
		return
	}
	review, err := restoreInterruptReview(pending.Interrupts, pending.Command.Answers)
	if err != nil {
		r.fail(fmt.Errorf("restore pending interrupt decisions: %w", err))
		return
	}
	r.prompts.adopt(review)
	r.deliverInterruptResume(review, pending.Command.Clone())
}

func (r *runs) reconcilePendingResume(pending workbench.PendingResume) {
	command := pending.Command
	r.tasks.operations.GoSession(resumeRecoveryOperation, false, func(ctx context.Context, lease operationLease) {
		_, err := mutation.ConfirmAdmitted(ctx, runtimeRecoveryBackoff, nil, func(ctx context.Context) (conversation.SegmentStream, error) {
			return runworkflow.OpenPendingResume(ctx, r.runtime, r.workbench, pending, r.replayPolicy)
		})
		if context.Cause(ctx) != nil {
			return
		}
		_ = r.tasks.deliver(ctx, func() {
			if !r.tasks.operations.Current(lease) || !r.tasks.operations.Release(lease) {
				return
			}
			if _, retired := errors.AsType[*conversation.RetiredOpeningError](err); err == nil || retired {
				r.settleAcknowledgedResume(command.CommandID)
				return
			}
			_, accepted := conversation.AcceptedMutationReceipt(err)
			if !accepted && !mutation.OutcomeUnknown(err) {
				err = errors.Join(err, r.workbench.RejectPendingResume(r.session.current.ID, command.CommandID))
			}
			r.link.message("reconcile earlier interrupt delivery: " + err.Error())
		})
	})
}

func sameInterrupts(left, right []conversation.Interrupt) bool {
	if len(left) != len(right) {
		return false
	}
	for index, item := range left {
		switch typed := item.(type) {
		case conversation.Approval:
			other, ok := right[index].(conversation.Approval)
			if !ok || !typed.Equal(other) {
				return false
			}
		case conversation.Question:
			other, ok := right[index].(conversation.Question)
			if !ok || !typed.Equal(other) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func (r *runs) restorePendingQueue() error {
	if err := r.queue.Restore(r.session.current.ID); err != nil {
		return fmt.Errorf("restore pending runs: %w", err)
	}
	r.syncQueue()
	return nil
}

func (r *runs) replayPendingRun(pending workbench.PendingRun) {
	entry, ok := r.queue.Dispatching(pending.Command.SessionID)
	if !ok || entry.CommandID != pending.Command.CommandID {
		r.fail(errors.New("replay pending run: dispatch reservation is unavailable"))
		return
	}
	r.startRun(entry.CommandID, entry.Message, entry.Options, "recovering queued prompt")
}

func (r *runs) reconcilePendingRun(pending workbench.PendingRun) {
	command := pending.Command
	r.tasks.operations.GoSession(pendingRunRecoveryOperation, false, func(ctx context.Context, lease operationLease) {
		opened, err := openStartRunWithBackoff(
			ctx, r.runtime, r.workbench, pending, r.replayPolicy, runtimeRecoveryBackoff,
		)
		if context.Cause(ctx) != nil {
			return
		}
		_ = r.tasks.deliver(ctx, func() {
			if !r.tasks.operations.Current(lease) || r.link.terminalClosed() || r.session.current.ID != command.SessionID ||
				!r.tasks.operations.Release(lease) {
				return
			}
			_, accepted := observedSegmentStream(opened, err)
			switch {
			case accepted:
				err = r.retireQueuedCommand(command.SessionID, command.CommandID)
			case errors.Is(err, conversation.ErrSessionHasActiveRun):
				err = r.queue.RequeueDispatch(command.SessionID, command.CommandID)
				if err == nil {
					r.syncQueue()
				}
			default:
				err = fmt.Errorf("reconcile pending run: %w", err)
			}
			if err != nil {
				r.link.message(err.Error())
			}
		})
	})
}
