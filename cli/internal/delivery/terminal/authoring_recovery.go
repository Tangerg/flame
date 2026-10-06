package terminal

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/cli/internal/application/workbench"
	"github.com/Tangerg/flame/cli/internal/domain/conversation"
)

// restoreSessionOutbox resumes durable runtime deliveries owned by the active
// session. It belongs to projection installation rather than process startup:
// every session can have an independent outbox, including one first visited
// after this process began.
func (a *app) restoreSessionOutbox() {
	a.restorePendingResume()
	a.restorePendingRuns()
}

func (a *app) restorePendingRuns() {
	pending := a.workbench.PendingRuns(a.session.current.ID)
	if len(pending) == 0 {
		return
	}
	if pending[0].State != workbench.PendingRunQueued {
		if _, err := pending[0].ReplayCommand(); err != nil {
			a.fail(fmt.Errorf("recover pending run input: %w", err))
			return
		}
	}
	if pending[0].State == workbench.PendingRunDispatching &&
		!a.replayPolicy.Replayable(pending[0].Replay) {
		a.fail(errors.New("recover pending run: replay guarantee expired or belongs to another runtime"))
		return
	}
	if pending[0].State == workbench.PendingRunCanceling &&
		(!a.replayPolicy.Replayable(pending[0].Replay) ||
			!a.replayPolicy.Replayable(pending[0].CancelReplay)) {
		a.fail(errors.New("recover pending run cancellation: replay guarantee expired or belongs to another runtime"))
		return
	}
	if err := a.restorePendingQueue(); err != nil {
		a.fail(err)
		return
	}
	if pending[0].State == workbench.PendingRunCanceling {
		a.reconcileCanceledStart(pending[0])
		return
	}
	if pending[0].State == workbench.PendingRunDispatching {
		if a.execution.conversation.Busy() {
			a.reconcilePendingRun(pending[0])
			return
		}
		a.replayPendingRun(pending[0])
		return
	}
	if !a.execution.conversation.Busy() {
		a.drainQueue()
	}
}

func (a *app) restorePendingResume() {
	pending, ok := a.workbench.PendingResume(a.session.current.ID)
	if !ok {
		return
	}
	if _, err := pending.ReplayCommand(); err != nil {
		a.fail(fmt.Errorf("recover interrupt input: %w", err))
		return
	}
	if !a.replayPolicy.SameStore(pending.Replay) {
		a.fail(errors.New("recover interrupt decisions: command belongs to another runtime"))
		return
	}
	if err := a.execution.conversation.ValidateInterruptReview(pending.Command.RunID, pending.Interrupts); err != nil {
		// The authoritative snapshot has advanced beyond this decision. Its exact
		// runtime outcome is therefore already visible and the local outbox can be
		// retired without replaying an obsolete command.
		if err := a.workbench.AcknowledgePendingResume(a.session.current.ID, pending.Command.CommandID); err != nil {
			a.fail(fmt.Errorf("retire settled interrupt decisions: %w", err))
		}
		return
	}
	if !a.replayPolicy.Replayable(pending.Replay) {
		replayGuard, err := a.replayPolicy.NewGuard()
		if err != nil {
			a.fail(fmt.Errorf("recover interrupt decisions: replace expired command: %w", err))
			return
		}
		requeued, err := a.workbench.RequeuePendingResume(a.session.current.ID, pending.Command.CommandID, replayGuard)
		if err != nil {
			a.fail(fmt.Errorf("recover interrupt decisions: replace expired command: %w", err))
			return
		}
		pending = requeued
		a.status.note("interrupt delivery expired · retrying safely")
	}
	review, err := restoreInterruptReview(pending.Interrupts, pending.Command.Answers)
	if err != nil {
		a.fail(fmt.Errorf("restore pending interrupt decisions: %w", err))
		return
	}
	a.dismissInterruptProjection()
	a.dialogs.interruptReview = review
	a.deliverInterruptResume(review, pending.Command.Clone(), pending.Replay)
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

func (a *app) restorePendingQueue() error {
	if err := a.queue.Restore(a.session.current.ID); err != nil {
		return fmt.Errorf("restore pending runs: %w", err)
	}
	a.syncQueue()
	return nil
}

func (a *app) replayPendingRun(pending workbench.PendingRun) {
	entry, ok := a.queue.Dispatching(pending.Command.SessionID)
	if !ok || entry.CommandID != pending.Command.CommandID {
		a.fail(errors.New("replay pending run: dispatch reservation is unavailable"))
		return
	}
	a.startRun(entry.CommandID, entry.Message, entry.Options, "recovering queued prompt")
}

func (a *app) reconcilePendingRun(pending workbench.PendingRun) {
	command := pending.Command
	activeRunID := a.execution.conversation.RunID()
	dispatcher := a.loop.Dispatcher()
	a.operations.GoSession(pendingRunRecoveryOperation, false, func(ctx context.Context, lease operationLease) {
		opened, err := openStartRunWithBackoff(
			ctx, a.runtime, command, pending.Replay, a.replayPolicy, runtimeRecoveryBackoff,
		)
		if context.Cause(ctx) != nil {
			return
		}
		_ = post(ctx, dispatcher, func() {
			if !a.operations.Current(lease) || a.closed || a.session.current.ID != command.SessionID ||
				!a.operations.Release(lease) {
				return
			}
			observed, accepted := observedSegmentStream(opened, err)
			switch {
			case accepted && observed.RunID == activeRunID:
				err = a.retireQueuedCommand(command.SessionID, command.CommandID)
			case accepted:
				err = fmt.Errorf("pending command %s opened run %s while session projects %s", command.CommandID, observed.RunID, activeRunID)
			case errors.Is(err, conversation.ErrSessionHasActiveRun):
				err = a.queue.RequeueDispatch(command.SessionID, command.CommandID)
				if err == nil {
					a.syncQueue()
				}
			default:
				err = fmt.Errorf("reconcile pending run: %w", err)
			}
			if err != nil {
				a.fail(err)
			}
		})
	})
}
