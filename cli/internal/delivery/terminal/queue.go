package terminal

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/keymap"
	"github.com/Tangerg/oolong/core/text"

	"github.com/Tangerg/flame/cli/internal/application/agent/promptqueue"
	"github.com/Tangerg/flame/cli/internal/application/agent/workbench"
	"github.com/Tangerg/flame/cli/internal/domain/agent"
	"github.com/Tangerg/flame/cli/internal/domain/commandreplay"
)

const (
	queueMinWidth = 28
	queueMaxRows  = 3
)

var errQueuedPromptDispatching = errors.New("queued prompt is already being sent")

var errQueuedPromptCanceling = errors.New("queued prompt is awaiting cancellation")

type queueView struct {
	theme    kit.Theme
	glyphs   kit.Glyphs
	snapshot promptqueue.Snapshot
}

func newQueueView(theme kit.Theme, glyphs kit.Glyphs) *queueView {
	return &queueView{theme: theme, glyphs: glyphs}
}

func (q *queueView) Set(snapshot promptqueue.Snapshot) {
	q.snapshot = snapshot
}

func (q *queueView) Measure(width int) int {
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

func queueEntryLabel(entry promptqueue.Entry) string {
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

func (a *app) enqueueDeferredPrompt(commandID agent.CommandID, message agent.Message) {
	_, err := a.queue.EnqueueCommand(commandID, a.session.current.ID, message, a.options)
	if err != nil {
		a.message(err.Error())
		return
	}
	a.resetComposer()
	a.operations.Cancel(completionOperation)
	a.completion.Dismiss()
	snapshot := a.syncQueue()
	label := "queued follow-up"
	if !a.execution.blocksAdmission() && a.operations.BlocksRunAdmission() {
		label = "queued behind runtime change"
	}
	a.message(fmt.Sprintf("%s · %d waiting", label, len(snapshot.Entries)))
}

func (a *app) drainQueue() bool {
	if a.closed {
		return false
	}
	if _, dispatching := a.queue.Dispatching(a.session.current.ID); dispatching {
		if a.execution.blocksAdmission() || a.execution.openingRunID == "" ||
			a.operations.Active(sessionChangeOperation) || a.operations.Active(pendingRunRecoveryOperation) ||
			a.operations.BlocksRunAdmission() {
			return false
		}
		if err := a.attemptQueuedDispatchSettlement(); err != nil {
			a.reportQueuedDispatchSettlementFailure(err)
			return false
		}
		a.execution.openingRunID = ""
	}
	if a.execution.blocksAdmission() ||
		a.operations.Active(sessionChangeOperation) || a.operations.Active(pendingRunRecoveryOperation) ||
		a.operations.BlocksRunAdmission() {
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

// settleQueuedDispatch closes the local side of an acknowledged StartRun.
// Durable history and outbox ownership move first; the in-memory reservation
// remains intact when persistence fails so later events or user activity can
// retry the same settlement without crossing the FIFO boundary.
func (a *app) attemptQueuedDispatchSettlement() error {
	entry, ok := a.queue.Dispatching(a.session.current.ID)
	if !ok {
		return nil
	}
	if a.workbench != nil {
		if pending, found := pendingRunByCommandID(a.workbench.PendingRuns(a.session.current.ID), entry.CommandID); found {
			if pending.State == workbench.PendingRunCanceling {
				return errQueuedPromptCanceling
			}
		}
	}
	return a.retireQueuedCommand(a.session.current.ID, entry.CommandID)
}

// retireQueuedCommand settles one exact StartRun command in both durable and
// live authoring projections. All accepted, recovered, and canceled opening
// paths use this method so prompt history cannot diverge by lifecycle route.
func (a *app) retireQueuedCommand(sessionID string, commandID agent.CommandID) error {
	entry, ok := a.queue.Dispatching(sessionID)
	pendingPresent := false
	if a.workbench != nil {
		_, pendingPresent = pendingRunByCommandID(a.workbench.PendingRuns(sessionID), commandID)
	}
	if !ok {
		present := slices.ContainsFunc(a.queue.Snapshot(sessionID).Entries, func(entry promptqueue.Entry) bool {
			return entry.CommandID == commandID
		})
		if !present && !pendingPresent {
			return nil
		}
		return errors.New("dispatching prompt command ownership was released before settlement")
	}
	if entry.CommandID != commandID {
		return errors.New("dispatching prompt command identity changed")
	}
	if a.workbench != nil {
		if pendingPresent {
			if err := a.workbench.AcknowledgePendingRun(sessionID, commandID); err != nil {
				return err
			}
		}
	}
	if _, err := a.queue.RetireCommand(sessionID, commandID); err != nil {
		return err
	}
	a.history.Add(entry.Message)
	a.reportWorkbenchIssue(workbenchRunOutbox, nil)
	a.syncQueue()
	return nil
}

func (a *app) settleQueuedDispatch() bool {
	if a.operations.Active(pendingRunSettlementOperation) {
		return false
	}
	if err := a.attemptQueuedDispatchSettlement(); err != nil {
		a.reportQueuedDispatchSettlementFailure(err)
		return false
	}
	return true
}

func (a *app) queuedDispatchCanceling() bool {
	if a.workbench == nil {
		return false
	}
	entry, ok := a.queue.Dispatching(a.session.current.ID)
	if !ok {
		return false
	}
	pending, found := pendingRunByCommandID(a.workbench.PendingRuns(a.session.current.ID), entry.CommandID)
	return found && pending.State == workbench.PendingRunCanceling
}

func (a *app) reportQueuedDispatchSettlementFailure(err error) {
	if errors.Is(err, errQueuedPromptCanceling) {
		return
	}
	a.reportWorkbenchIssue(workbenchRunOutbox, err)
	a.message("could not settle acknowledged run start: " + err.Error())
	a.retryQueuedDispatchSettlement()
}

func (a *app) retryQueuedDispatchSettlement() {
	if a.workbench == nil {
		return
	}
	runID := a.execution.openingRunID
	a.retryAuthoringSettlement(
		pendingRunSettlementOperation,
		a.attemptQueuedDispatchSettlement,
		func() { a.finishQueuedSettlementRecovery(runID) },
	)
}

// retryCanceledRuntimeOwnership is the cancellation counterpart to ordinary
// acknowledgement settlement. It retries both a pending HITL decision and the
// exact opening command, because either can remain after a partial state write.
func (a *app) retryCanceledRuntimeOwnership(runID string, commandID agent.CommandID) {
	a.retryAuthoringSettlement(
		ownershipSettlementOperation,
		func() error { return a.retireCanceledRuntimeOwnership(runID, commandID) },
		func() { a.finishCanceledRuntimeOwnershipRecovery(runID) },
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

func (a *app) finishQueuedSettlementRecovery(runID string) {
	if a.execution.openingRunID == runID {
		a.execution.openingRunID = ""
	}
	a.finishAuthoringSettlementRecovery()
}

// A canceled ownership retry may outlive the queue drain that starts the next
// run. It must only clear the opening identity it originally owned; otherwise
// a late local disk recovery could detach the successor's lifecycle.
func (a *app) finishCanceledRuntimeOwnershipRecovery(runID string) {
	if a.execution.openingRunID == runID {
		a.execution.openingRunID = ""
	}
	a.reportWorkbenchIssue(workbenchCancellationOwnership, nil)
	a.finishAuthoringSettlementRecovery()
}

func (a *app) finishAuthoringSettlementRecovery() {
	if a.execution.blocksAdmission() || a.drainQueue() {
		return
	}
	if a.execution.conversation.Outcome().Status != "" {
		a.settleCurrentRunStatus()
		a.syncAnimation()
	}
}

func (a *app) syncQueue() promptqueue.Snapshot {
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

func (a *app) persistQueuedRuns() error {
	if a.workbench == nil {
		return nil
	}
	state := a.queue.State(a.session.current.ID)
	entries := state.Entries
	persisted := a.workbench.PendingRuns(a.session.current.ID)
	commands := make([]workbench.PendingRun, 0, len(entries))
	dispatchingID, hasDispatch := state.DispatchingID()
	for _, entry := range entries {
		pendingState := workbench.PendingRunQueued
		if hasDispatch && entry.ID == dispatchingID {
			pending, ok := pendingRunByCommandID(persisted, entry.CommandID)
			if !ok {
				// StartRun was already acknowledged. The UI queue retains the
				// entry until SegmentStarted arrives, but it no longer belongs in
				// the durable authoring outbox.
				continue
			}
			pendingState = pending.State
			commands = append(commands, workbench.PendingRun{
				State: pendingState,
				Command: agent.StartRun{
					CommandID: entry.CommandID, SessionID: entry.SessionID,
					Message: entry.Message.Clone(), Options: entry.Options.Clone(),
				},
				Replay: pending.Replay, CancelCommandID: pending.CancelCommandID,
				CancelReplay: pending.CancelReplay,
			})
			continue
		}
		commands = append(commands, workbench.PendingRun{
			State: workbench.PendingRunQueued, Replay: commandreplay.UnprotectedGuard(),
			CancelReplay: commandreplay.UnprotectedGuard(),
			Command: agent.StartRun{
				CommandID: entry.CommandID, SessionID: entry.SessionID,
				Message: entry.Message.Clone(), Options: entry.Options.Clone(),
			},
		})
		commands[len(commands)-1].State = pendingState
	}
	return a.workbench.SavePendingRuns(a.session.current.ID, commands)
}

// commitQueueMutation keeps the in-memory queue and durable authoring outbox
// on the same side of every edit. Runtime control starts only after this
// transaction succeeds, so a failed disk write cannot silently change FIFO
// order, content, or command identity for the live process alone.
func (a *app) commitQueueMutation(mutate func() error) error {
	before := a.queue.State(a.session.current.ID)
	if err := mutate(); err != nil {
		return a.rollbackQueueMutation(before, err)
	}
	if err := a.persistQueuedRuns(); err != nil {
		return a.rollbackQueueMutation(before, err)
	}
	a.syncQueue()
	return nil
}

func (a *app) rollbackQueueMutation(before promptqueue.State, cause error) error {
	if err := a.queue.RestoreState(a.session.current.ID, before); err != nil {
		return errors.Join(cause, fmt.Errorf("restore prompt queue: %w", err))
	}
	a.syncQueue()
	return cause
}

func pendingRunByCommandID(pending []workbench.PendingRun, commandID agent.CommandID) (workbench.PendingRun, bool) {
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

func (a *app) holdQueuedPrompt(entry promptqueue.Entry) error {
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

func (a *app) saveQueuedPrompt(entry promptqueue.Entry, message agent.Message, sendNow bool) error {
	if entry.SessionID != a.session.current.ID {
		return errors.New("queued prompt belongs to another session")
	}
	if dispatching, ok := a.queue.Dispatching(entry.SessionID); ok && dispatching.ID == entry.ID {
		return errQueuedPromptDispatching
	}
	if err := a.commitQueueMutation(func() error {
		if err := a.queue.Update(entry.SessionID, entry.ID, message); err != nil {
			return err
		}
		return a.queue.Release(entry.SessionID, entry.ID)
	}); err != nil {
		return err
	}
	if sendNow {
		return a.sendQueuedNow(entry.ID)
	}
	a.drainQueue()
	return nil
}

func (a *app) releaseQueuedPrompt(entry promptqueue.Entry) error {
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

func (a *app) removeQueuedPrompt(id promptqueue.EntryID) error {
	if entry, ok := a.queue.Dispatching(a.session.current.ID); ok && id == entry.ID {
		return errQueuedPromptDispatching
	}
	if err := a.commitQueueMutation(func() error {
		_, err := a.queue.Remove(a.session.current.ID, id)
		return err
	}); err != nil {
		return err
	}
	snapshot := a.queue.Snapshot(a.session.current.ID)
	if len(snapshot.Entries) == 0 {
		a.message("queue is empty")
	}
	return nil
}

func (a *app) moveQueuedPrompt(id promptqueue.EntryID, offset int) error {
	if entry, ok := a.queue.Dispatching(a.session.current.ID); ok && id == entry.ID {
		return errQueuedPromptDispatching
	}
	if err := a.commitQueueMutation(func() error {
		return a.queue.Move(a.session.current.ID, id, offset)
	}); err != nil {
		return err
	}
	return nil
}

// sendQueuedNow persists priority in the queue before touching the active run.
// Cancellation and dispatch therefore remain resumable if either runtime control
// call is delayed or fails: the promoted entry is still the next FIFO item.
func (a *app) sendQueuedNow(id promptqueue.EntryID) error {
	if entry, ok := a.queue.Dispatching(a.session.current.ID); ok && id == entry.ID {
		return errQueuedPromptDispatching
	}
	if a.operations.Active(sessionChangeOperation) {
		return errors.New("wait for the current session change to finish")
	}
	if !a.execution.blocksAdmission() && a.operations.BlocksRunAdmission() {
		return errors.New("wait for the pending runtime change before sending a queued prompt")
	}
	if err := a.commitQueueMutation(func() error {
		return a.queue.Promote(a.session.current.ID, id)
	}); err != nil {
		return err
	}
	if a.execution.blocksAdmission() {
		a.cancel()
		return nil
	}
	if !a.drainQueue() {
		return errors.New("queued prompt could not be dispatched")
	}
	return nil
}
