package terminal

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/flame/apps/cli/internal/application/workbench"
	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/keymap"
	"github.com/Tangerg/oolong/core/text"
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
		view.Text(0, index+1, q.glyphs.Vertical, q.theme.Divider)
		label := fmt.Sprintf("%s %d. %s", marker, index+1, queueEntryLabel(entry))
		view.Text(2, index+1, text.Truncate(label, max(width-2, 1), q.glyphs.Ellipsis), style)
	}
	if visible < capacity && visible < len(q.snapshot.Entries) {
		remaining := len(q.snapshot.Entries) - visible
		row := visible + 1
		view.Text(0, row, q.glyphs.Vertical, q.theme.Divider)
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

func (a *app) enqueueDeferredPrompt() {
	a.resetComposer()
	a.operations.Cancel(completionOperation)
	a.completion.Dismiss()
	snapshot := a.syncQueue()
	label := "queued follow-up"
	if a.runtimeChangeBlocksRunAdmission() {
		label = "queued behind runtime change"
	}
	a.message(fmt.Sprintf("%s · %d waiting", label, len(snapshot.Entries)))
}

func (a *app) drainQueue() bool {
	if a.closed {
		return false
	}
	if _, dispatching := a.queue.Dispatching(a.session.current.ID); dispatching {
		if a.runAdmissionBlocked() || a.execution.opening == nil ||
			a.operations.Active(sessionChangeOperation) || a.operations.Active(pendingRunRecoveryOperation) {
			return false
		}
		if err := a.attemptQueuedDispatchSettlement(a.execution.opening.CommandID); err != nil {
			a.reportQueuedDispatchSettlementFailure(a.execution.opening.CommandID, err)
			return false
		}
		a.execution.opening = nil
	}
	if a.runAdmissionBlocked() ||
		a.operations.Active(sessionChangeOperation) || a.operations.Active(pendingRunRecoveryOperation) {
		return false
	}
	entry, ok := a.queue.BeginDispatch(a.session.current.ID)
	if !ok {
		return false
	}
	a.syncQueue()
	if !a.startRun(entry.CommandID, entry.Message, entry.Options, "starting queued follow-up") {
		a.queue.ReleaseDispatch(a.session.current.ID)
		a.syncQueue()
		return false
	}
	return true
}

func (a *app) attemptQueuedDispatchSettlement(commandID authoring.CommandID) error {
	entry, retired, err := a.queue.SettleDispatch(a.session.current.ID, commandID)
	if err != nil {
		return err
	}
	a.presentQueuedRetirement(entry, retired)
	return nil
}

func (a *app) retireQueuedCommand(sessionID string, commandID authoring.CommandID) error {
	entry, retired, err := a.queue.Retire(sessionID, commandID)
	if err != nil {
		return err
	}
	a.presentQueuedRetirement(entry, retired)
	return nil
}

func (a *app) presentQueuedRetirement(entry authoring.QueueEntry, retired bool) {
	if !retired {
		return
	}
	a.history.Add(entry.Message)
	a.reportWorkbenchIssue(workbenchRunOutbox, nil)
	a.syncQueue()
}

func (a *app) settleQueuedDispatch(commandID authoring.CommandID) bool {
	if a.operations.Active(pendingRunSettlementOperation) {
		return false
	}
	if err := a.attemptQueuedDispatchSettlement(commandID); err != nil {
		a.reportQueuedDispatchSettlementFailure(commandID, err)
		return false
	}
	return true
}

func (a *app) queuedDispatchCanceling() bool {
	return a.queue.DispatchCanceling(a.session.current.ID)
}

func (a *app) reportQueuedDispatchSettlementFailure(commandID authoring.CommandID, err error) {
	if errors.Is(err, workbench.ErrDispatchCanceling) {
		return
	}
	a.reportWorkbenchIssue(workbenchRunOutbox, err)
	a.message("could not settle acknowledged run start: " + err.Error())
	a.retryQueuedDispatchSettlement(commandID)
}

func (a *app) retryQueuedDispatchSettlement(commandID authoring.CommandID) {
	receipt := a.execution.opening
	a.retryAuthoringSettlement(
		pendingRunSettlementOperation,
		func() error { return a.attemptQueuedDispatchSettlement(commandID) },
		func() { a.finishQueuedSettlementRecovery(receipt) },
	)
}

// retryCanceledRuntimeOwnership is the cancellation counterpart to ordinary
// acknowledgement settlement. It retries both a pending HITL decision and the
// exact opening command, because either can remain after a partial state write.
func (a *app) retryCanceledRuntimeOwnership(runID string, commandID authoring.CommandID) {
	a.retryAuthoringSettlement(
		ownershipSettlementOperation,
		func() error { return a.retireCanceledRuntimeOwnership(runID, commandID) },
		func() { a.finishCanceledRuntimeOwnershipRecovery(runID, commandID) },
	)
}

// retryAuthoringSettlement owns transient local-commit recovery for the active
// session. The settlement itself always runs on the UI thread: it may update
// both a durable workbench aggregate and its in-memory projection atomically
// from the application's point of view.
func (a *app) retryAuthoringSettlement(slot operationSlot, settle func() error, finish func()) {
	if settle == nil || a.operations.Active(slot) {
		return
	}
	sessionID := a.session.current.ID
	dispatcher := a.loop.Dispatcher()
	a.operations.GoSessionSettlement(slot, false, func(ctx context.Context, lease operationLease) {
		for failures := 1; ; failures++ {
			if err := runtimeRecoveryBackoff.Wait(ctx, failures); err != nil {
				return
			}
			completed := false
			if err := post(ctx, dispatcher, func() {
				if !a.operations.Current(lease) || a.closed || a.session.current.ID != sessionID {
					return
				}
				if settle() != nil || !a.operations.Release(lease) {
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

func (a *app) finishQueuedSettlementRecovery(receipt *openingReceipt) {
	if a.execution.opening == receipt {
		a.execution.opening = nil
	}
	a.finishAuthoringSettlementRecovery()
}

// A canceled ownership retry may outlive the queue drain that starts the next
// run. It must only clear the opening identity it originally owned; otherwise
// a late local disk recovery could detach the successor's lifecycle.
func (a *app) finishCanceledRuntimeOwnershipRecovery(runID string, commandID authoring.CommandID) {
	if a.execution.opening != nil && a.execution.opening.RunID == runID && a.execution.opening.CommandID == commandID {
		a.execution.opening = nil
	}
	a.reportWorkbenchIssue(workbenchCancellationOwnership, nil)
	a.finishAuthoringSettlementRecovery()
}

func (a *app) finishAuthoringSettlementRecovery() {
	if a.runAdmissionBlocked() || a.drainQueue() {
		return
	}
	if a.execution.conversation.Outcome().Status != "" {
		a.settleCurrentRunStatus()
		a.syncAnimation()
	}
}

func (a *app) syncQueue() authoring.QueueSnapshot {
	snapshot := a.queue.Snapshot(a.session.current.ID)
	a.queueView.Set(snapshot)
	a.prompt.SetQueued(len(snapshot.Entries))
	if a.queueDrawer != nil {
		a.queueDrawer.Set(snapshot)
	}
	if a.dialogs.queueDialog != nil {
		a.dialogs.queueDialog.SetTitle("Queue · " + countedNoun(len(snapshot.Entries), "prompt"))
		a.dialogs.queueDialog.SetDescription(fmt.Sprintf("Manage %s waiting behind the current turn", countedNoun(len(snapshot.Entries), "prompt")))
		if len(snapshot.Entries) == 0 && a.dialogs.queueDialog.Open() {
			a.dialogs.queueDialog.Dismiss()
		}
	}
	return snapshot
}

func pendingRunByCommandID(pending []workbench.PendingRun, commandID authoring.CommandID) (workbench.PendingRun, bool) {
	for _, candidate := range pending {
		if candidate.Command.CommandID == commandID {
			return candidate, true
		}
	}
	return workbench.PendingRun{}, false
}

func (a *app) ShowQueue() {
	snapshot := a.syncQueue()
	if len(snapshot.Entries) == 0 {
		a.message("queue is empty")
		return
	}
	a.queueDrawer.ResetNotice()
	a.queueDrawer.lifecycle.renew()
	a.dialogs.queueDialog.Show()
	a.message(fmt.Sprintf("queue · %d waiting", len(snapshot.Entries)))
}

func (a *app) buildQueueDrawer(theme kit.Theme, glyphs kit.Glyphs, keys *keymap.Map) {
	drawer := newQueueDrawer(theme, glyphs, keys, a.loop.Clipboard())
	dialog := headless.NewDialog(headless.DialogConfig{Stack: &a.stack, Title: "Queue", Content: drawer})
	drawer.SetActions(queueDrawerActions{
		BeginEdit:  a.holdQueuedPrompt,
		SaveEdit:   a.saveQueuedPrompt,
		CancelEdit: a.releaseQueuedPrompt,
		Remove:     a.removeQueuedPrompt,
		Move:       a.moveQueuedPrompt,
		SendNow:    a.sendQueuedNow,
		Dismiss:    dialog.Dismiss,
	})
	drawer.lifecycle.bind(dialog.Open)
	a.queueDrawer = drawer
	a.dialogs.queueDialog = dialog
}

func (a *app) holdQueuedPrompt(entry authoring.QueueEntry) error {
	if entry.SessionID != a.session.current.ID {
		return errors.New("queued prompt belongs to another session")
	}
	if dispatching, ok := a.queue.Dispatching(entry.SessionID); ok && dispatching.ID == entry.ID {
		return errQueuedPromptDispatching
	}
	if err := a.queue.Hold(entry.SessionID, entry.ID); err != nil {
		return err
	}
	a.syncQueue()
	return nil
}

func (a *app) saveQueuedPrompt(entry authoring.QueueEntry, message authoring.Message, sendNow bool) error {
	if entry.SessionID != a.session.current.ID {
		return errors.New("queued prompt belongs to another session")
	}
	if dispatching, ok := a.queue.Dispatching(entry.SessionID); ok && dispatching.ID == entry.ID {
		return errQueuedPromptDispatching
	}
	if err := a.queue.Update(entry.SessionID, entry.ID, message); err != nil {
		return err
	}
	a.syncQueue()
	if sendNow {
		return a.sendQueuedNow(entry.ID)
	}
	a.drainQueue()
	return nil
}

func (a *app) releaseQueuedPrompt(entry authoring.QueueEntry) error {
	if err := a.queue.Release(entry.SessionID, entry.ID); err != nil {
		return err
	}
	if entry.SessionID != a.session.current.ID {
		return nil
	}
	a.syncQueue()
	a.drainQueue()
	return nil
}

func (a *app) removeQueuedPrompt(id authoring.QueueEntryID) error {
	if entry, ok := a.queue.Dispatching(a.session.current.ID); ok && id == entry.ID {
		return errQueuedPromptDispatching
	}
	if _, err := a.queue.Remove(a.session.current.ID, id); err != nil {
		return err
	}
	snapshot := a.syncQueue()
	if len(snapshot.Entries) == 0 {
		a.message("queue is empty")
	}
	return nil
}

func (a *app) moveQueuedPrompt(id authoring.QueueEntryID, offset int) error {
	if entry, ok := a.queue.Dispatching(a.session.current.ID); ok && id == entry.ID {
		return errQueuedPromptDispatching
	}
	if err := a.queue.Move(a.session.current.ID, id, offset); err != nil {
		return err
	}
	a.syncQueue()
	return nil
}

// sendQueuedNow persists priority in the queue before touching the active run.
// Cancellation and dispatch therefore remain resumable if either runtime control
// call is delayed or fails: the promoted entry is still the next FIFO item.
func (a *app) sendQueuedNow(id authoring.QueueEntryID) error {
	if entry, ok := a.queue.Dispatching(a.session.current.ID); ok && id == entry.ID {
		return errQueuedPromptDispatching
	}
	if a.operations.Active(sessionChangeOperation) {
		return errors.New("wait for the current session change to finish")
	}
	if !a.execution.blocksAdmission() && a.runtimeChangeBlocksRunAdmission() {
		return errors.New("wait for the pending runtime change before sending a queued prompt")
	}
	if err := a.queue.Promote(a.session.current.ID, id); err != nil {
		return err
	}
	a.syncQueue()
	if a.execution.blocksAdmission() {
		a.cancel()
		return nil
	}
	if !a.drainQueue() {
		return errors.New("queued prompt could not be dispatched")
	}
	return nil
}
