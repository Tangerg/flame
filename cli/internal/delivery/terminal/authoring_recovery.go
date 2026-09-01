package terminal

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/cli/internal/application/agent/workbench"
	"github.com/Tangerg/flame/cli/internal/domain/agent"
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
	if a.workbench == nil {
		return
	}
	pending := a.workbench.PendingRuns(a.session.current.ID)
	if len(pending) == 0 {
		return
	}
	if pending[0].State == workbench.PendingRunDispatching &&
		!commandReplaySafe(pending[0].Replay, a.runtimeProfile) {
		a.fail(errors.New("recover pending run: replay guarantee expired or belongs to another runtime"))
		return
	}
	if pending[0].State == workbench.PendingRunCanceling &&
		(!commandReplaySafe(pending[0].Replay, a.runtimeProfile) ||
			!commandReplaySafe(pending[0].CancelReplay, a.runtimeProfile)) {
		a.fail(errors.New("recover pending run cancellation: replay guarantee expired or belongs to another runtime"))
		return
	}
	if err := a.restorePendingQueue(pending); err != nil {
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
	if a.workbench == nil {
		return
	}
	pending, ok := a.workbench.PendingResume(a.session.current.ID)
	if !ok {
		return
	}
	if !commandReplayStoreMatches(pending.Replay, a.runtimeProfile) {
		a.fail(errors.New("recover interaction decisions: command belongs to another runtime"))
		return
	}
	if a.execution.conversation.Phase() != agent.ConversationWaiting || a.execution.conversation.RunID() != pending.Command.RunID ||
		!sameInteractions(a.execution.conversation.Interactions(), pending.Interactions) {
		// The authoritative snapshot has advanced beyond this decision. Its exact
		// runtime outcome is therefore already visible and the local outbox can be
		// retired without replaying an obsolete command.
		if err := a.workbench.AcknowledgePendingResume(a.session.current.ID, pending.Command.CommandID); err != nil {
			a.fail(fmt.Errorf("retire settled interaction decisions: %w", err))
		}
		return
	}
	if !commandReplaySafe(pending.Replay, a.runtimeProfile) {
		requeued, err := a.workbench.RequeuePendingResume(
			a.session.current.ID, pending.Command.CommandID, commandReplayGuard(a.runtimeProfile),
		)
		if err != nil {
			a.fail(fmt.Errorf("recover interaction decisions: replace expired command: %w", err))
			return
		}
		pending = requeued
		a.status.note("interaction delivery expired · retrying safely")
	}
	review, err := restoreInteractionReview(pending.Interactions, pending.Command.Answers)
	if err != nil {
		a.fail(fmt.Errorf("restore pending interaction decisions: %w", err))
		return
	}
	a.dismissInteractionProjection()
	a.dialogs.interactionReview = review
	a.deliverInteractionResume(review, pending.Command.Clone(), pending.Replay)
}

func sameInteractions(left, right []agent.Interaction) bool {
	if len(left) != len(right) {
		return false
	}
	for index, item := range left {
		switch typed := item.(type) {
		case agent.Approval:
			other, ok := right[index].(agent.Approval)
			if !ok || !typed.Equal(other) {
				return false
			}
		case agent.Question:
			other, ok := right[index].(agent.Question)
			if !ok || !typed.Equal(other) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func (a *app) restorePendingQueue(pending []workbench.PendingRun) error {
	commands := make([]agent.StartRun, 0, len(pending))
	for _, entry := range pending {
		commands = append(commands, entry.Command.Clone())
	}
	var dispatching agent.CommandID
	if len(pending) > 0 && pending[0].State != workbench.PendingRunQueued {
		dispatching = pending[0].Command.CommandID
	}
	if err := a.queue.Restore(a.session.current.ID, commands, dispatching); err != nil {
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
			ctx, a.runtime, command, pending.Replay, a.runtimeProfile, runtimeRecoveryBackoff,
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
			case errors.Is(err, agent.ErrSessionHasActiveRun):
				_, err = a.workbench.RequeuePendingRun(command.SessionID, command.CommandID)
			default:
				err = fmt.Errorf("reconcile pending run: %w", err)
			}
			if err != nil {
				a.fail(err)
				return
			}
			if err := a.restorePendingQueue(a.workbench.PendingRuns(command.SessionID)); err != nil {
				a.fail(err)
			}
		})
	})
}
