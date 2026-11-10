package terminal

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/flame/apps/cli/internal/application/workbench"
	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/text"
	"github.com/Tangerg/oolong/ui/kit"
)

const (
	queueMinWidth = 28
	queueMaxRows  = 3
)

var errQueuedPromptDispatching = errors.New("queued prompt is already being sent")

type queueView struct {
	theme    kit.Theme
	glyphs   kit.Glyphs
	snapshot authoring.QueueSnapshot
}

func newQueueView(theme kit.Theme, glyphs kit.Glyphs) *queueView {
	return &queueView{theme: theme, glyphs: glyphs}
}

func (q *queueView) Set(snapshot authoring.QueueSnapshot) {
	q.snapshot = snapshot
}

func (q *queueView) HeightForWidth(width int) int {
	if width < queueMinWidth || len(q.snapshot.Entries) == 0 {
		return 0
	}
	return min(len(q.snapshot.Entries)+1, queueMaxRows)
}

func (q *queueView) Draw(view grid.View) {
	width, height := view.Size()
	if width < queueMinWidth || height <= 0 || len(q.snapshot.Entries) == 0 {
		return
	}
	header := q.glyphs.Expanded + " Queue"
	view.Text(0, 0, header, q.theme.Heading)
	count := fmt.Sprintf("%d queued", len(q.snapshot.Entries))
	if text.Width(header)+text.Width(count)+2 <= width {
		view.Text(width-text.Width(count), 0, count, q.theme.Subtle)
	}

	capacity := height - 1
	visible := min(capacity, len(q.snapshot.Entries))
	if len(q.snapshot.Entries) > capacity && capacity > 1 {
		visible--
	}
	for index := range visible {
		entry := q.snapshot.Entries[index]
		style, marker := q.theme.Muted, q.glyphs.Free
		if index == 0 {
			style, marker = q.theme.Accent, q.glyphs.Marker
		}
		view.Text(0, index+1, q.glyphs.Vertical.String(), q.theme.Divider)
		label := fmt.Sprintf("%s %d. %s", marker, index+1, queueEntryLabel(entry))
		view.Text(2, index+1, text.Truncate(label, max(width-2, 1), q.glyphs.Ellipsis), style)
	}
	if visible < capacity && visible < len(q.snapshot.Entries) {
		remaining := len(q.snapshot.Entries) - visible
		row := visible + 1
		view.Text(0, row, q.glyphs.Vertical.String(), q.theme.Divider)
		view.Text(2, row, fmt.Sprintf("%s %d more", q.glyphs.Ellipsis, remaining), q.theme.Subtle)
	}
}

func queueEntryLabel(entry authoring.QueueEntry) string {
	label := strings.TrimSpace(entry.Message.Text)
	if line, _, ok := strings.Cut(label, "\n"); ok {
		label = strings.TrimSpace(line)
	}
	attachments := len(entry.Message.Attachments)
	if label == "" && attachments > 0 {
		label = "@" + entry.Message.Attachments[0].Name
		attachments--
	}
	if attachments > 0 {
		label += " · " + countedNoun(attachments, "attachment")
	}
	return label
}

func countedNoun(count int, noun string) string {
	if count == 1 {
		return fmt.Sprintf("%d %s", count, noun)
	}
	return fmt.Sprintf("%d %ss", count, noun)
}

func (r *runs) enqueueDeferredPrompt() {
	r.composer.resetComposer()
	r.tasks.operations.Cancel(completionOperation)
	r.link.dismissCompletion()
	snapshot := r.syncQueue()
	label := "queued follow-up"
	if r.runtimeChangeBlocksRunAdmission() {
		label = "queued behind runtime change"
	}
	r.link.message(fmt.Sprintf("%s · %d waiting", label, len(snapshot.Entries)))
}

func (r *runs) drainQueue() bool {
	if r.link.terminalClosed() {
		return false
	}
	if _, dispatching := r.queue.Dispatching(r.session.current.ID); dispatching {
		if r.runAdmissionBlocked() || r.execution.opening == nil ||
			r.tasks.operations.Active(sessionChangeOperation) || r.tasks.operations.Active(pendingRunRecoveryOperation) {
			return false
		}
		if err := r.attemptQueuedDispatchSettlement(r.execution.opening.CommandID); err != nil {
			r.reportQueuedDispatchSettlementFailure(r.execution.opening.CommandID, err)
			return false
		}
		r.execution.opening = nil
	}
	if r.runAdmissionBlocked() ||
		r.tasks.operations.Active(sessionChangeOperation) || r.tasks.operations.Active(pendingRunRecoveryOperation) {
		return false
	}
	entry, ok := r.queue.BeginDispatch(r.session.current.ID)
	if !ok {
		return false
	}
	r.syncQueue()
	if !r.startRun(entry.CommandID, entry.Message, entry.Options, "starting queued follow-up") {
		r.queue.ReleaseDispatch(r.session.current.ID)
		r.syncQueue()
		return false
	}
	return true
}

func (r *runs) attemptQueuedDispatchSettlement(commandID authoring.CommandID) error {
	entry, retired, err := r.queue.SettleDispatch(r.session.current.ID, commandID)
	if err != nil {
		return err
	}
	r.presentQueuedRetirement(entry, retired)
	return nil
}

func (r *runs) retireQueuedCommand(sessionID string, commandID authoring.CommandID) error {
	entry, retired, err := r.queue.Retire(sessionID, commandID)
	if err != nil {
		return err
	}
	r.presentQueuedRetirement(entry, retired)
	return nil
}

func (r *runs) presentQueuedRetirement(entry authoring.QueueEntry, retired bool) {
	if !retired {
		return
	}
	r.composer.history.Add(entry.Message)
	r.report(workbenchRunOutbox, nil)
	r.syncQueue()
}

func (r *runs) settleQueuedDispatch(commandID authoring.CommandID) bool {
	if r.tasks.operations.Active(pendingRunSettlementOperation) {
		return false
	}
	if err := r.attemptQueuedDispatchSettlement(commandID); err != nil {
		r.reportQueuedDispatchSettlementFailure(commandID, err)
		return false
	}
	return true
}

func (r *runs) queuedDispatchCanceling() bool {
	return r.queue.DispatchCanceling(r.session.current.ID)
}

func (r *runs) reportQueuedDispatchSettlementFailure(commandID authoring.CommandID, err error) {
	if errors.Is(err, workbench.ErrDispatchCanceling) {
		return
	}
	r.report(workbenchRunOutbox, err)
	r.link.message("could not settle acknowledged run start: " + err.Error())
	r.retryQueuedDispatchSettlement(commandID)
}

func (r *runs) retryQueuedDispatchSettlement(commandID authoring.CommandID) {
	receipt := r.execution.opening
	r.retryAuthoringSettlement(
		pendingRunSettlementOperation,
		func() error { return r.attemptQueuedDispatchSettlement(commandID) },
		func() { r.finishQueuedSettlementRecovery(receipt) },
	)
}

// retryCanceledRuntimeOwnership is the cancellation counterpart to ordinary
// acknowledgement settlement. It retries both a pending HITL decision and the
// exact opening command, because either can remain after a partial state write.
func (r *runs) retryCanceledRuntimeOwnership(runID string, commandID authoring.CommandID) {
	r.retryAuthoringSettlement(
		ownershipSettlementOperation,
		func() error { return r.retireCanceledRuntimeOwnership(runID, commandID) },
		func() { r.finishCanceledRuntimeOwnershipRecovery(runID, commandID) },
	)
}

// retryAuthoringSettlement owns transient local-commit recovery for the active
// session. The settlement itself always runs on the UI thread: it may update
// both a durable workbench aggregate and its in-memory projection atomically
// from the application's point of view.
func (r *runs) retryAuthoringSettlement(slot operationSlot, settle func() error, finish func()) {
	if settle == nil || r.tasks.operations.Active(slot) {
		return
	}
	sessionID := r.session.current.ID
	r.tasks.operations.GoSessionSettlement(slot, false, func(ctx context.Context, lease operationLease) {
		for failures := 1; ; failures++ {
			if err := runtimeRecoveryBackoff.Wait(ctx, failures); err != nil {
				return
			}
			completed := false
			if err := r.tasks.deliver(ctx, func() {
				if !r.tasks.operations.Current(lease) || r.link.terminalClosed() || r.session.current.ID != sessionID {
					return
				}
				if settle() != nil || !r.tasks.operations.Release(lease) {
					return
				}
				completed = true
				if finish != nil {
					finish()
				}
			}); err != nil || completed {
				return
			}
		}
	})
}

func (r *runs) finishQueuedSettlementRecovery(receipt *openingReceipt) {
	if r.execution.opening == receipt {
		r.execution.opening = nil
	}
	r.finishAuthoringSettlementRecovery()
}

// A canceled ownership retry may outlive the queue drain that starts the next
// run. It must only clear the opening identity it originally owned; otherwise
// a late local disk recovery could detach the successor's lifecycle.
func (r *runs) finishCanceledRuntimeOwnershipRecovery(runID string, commandID authoring.CommandID) {
	if r.execution.opening != nil && r.execution.opening.RunID == runID && r.execution.opening.CommandID == commandID {
		r.execution.opening = nil
	}
	r.report(workbenchCancellationOwnership, nil)
	r.finishAuthoringSettlementRecovery()
}

func (r *runs) finishAuthoringSettlementRecovery() {
	if r.runAdmissionBlocked() || r.drainQueue() {
		return
	}
	if r.execution.conversation.Outcome().Status != "" {
		r.settleCurrentRunStatus()
		r.syncAnimation()
	}
}

func pendingRunByCommandID(pending []workbench.PendingRun, commandID authoring.CommandID) (workbench.PendingRun, bool) {
	for _, candidate := range pending {
		if candidate.Command.CommandID == commandID {
			return candidate, true
		}
	}
	return workbench.PendingRun{}, false
}

// queueSendBlocked reports why a queued prompt cannot be promoted now.
func (r *runs) queueSendBlocked() error {
	if r.tasks.operations.Active(sessionChangeOperation) {
		return errors.New("wait for the current session change to finish")
	}
	if !r.execution.blocksAdmission() && r.runtimeChangeBlocksRunAdmission() {
		return errors.New("wait for the pending runtime change before sending a queued prompt")
	}
	return nil
}

// dispatchPromoted sends the promoted entry: an active Run is canceled first,
// and an idle session dispatches it at once.
func (r *runs) dispatchPromoted() error {
	if r.execution.blocksAdmission() {
		r.cancel()
		return nil
	}
	if !r.drainQueue() {
		return errors.New("queued prompt could not be dispatched")
	}
	return nil
}

func (r *runs) syncQueue() authoring.QueueSnapshot {
	return r.queueEditor.sync()
}
