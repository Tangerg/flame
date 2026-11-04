package terminal

import (
	"fmt"
	"time"

	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/keymap"
)

func (a *app) Handle(event input.Event) bool {
	a.observeAttention(event)
	matched, handled := a.matchConfiguredAction(event)
	if handled {
		return true
	}
	if !matched {
		a.disarmConfirmation(event, "")
	}
	return a.handleUnboundEvent(event)
}

func (a *app) matchConfiguredAction(event input.Event) (matched, handled bool) {
	key, ok := event.(input.Key)
	if !ok || !key.Down() {
		return false, false
	}
	modal := a.stack.hasModal()
	if modal {
		a.prompt.SetPendingKeySequence("")
	}
	matched, handled, pending := a.keys.route(key, modal, a.stack.Element().KeyboardLease().Valid, func(action keymap.Action) bool {
		a.disarmConfirmation(event, action)
		return a.handleConfiguredAction(event, action)
	})
	if !modal {
		a.prompt.SetPendingKeySequence(pending)
	}
	return matched, handled
}

func (a *app) handleConfiguredAction(event input.Event, action keymap.Action) bool {
	if action == quitApp {
		a.handleQuit()
		return true
	}
	// Oolong modal stacks intentionally consume every key so input cannot leak
	// into covered content. Blocking runtime interrupts are the exception at
	// the product-policy layer: their cancel action must resolve the interrupt,
	// not disappear into the modal boundary.
	if action == cancelRun && a.prompts.answering() {
		a.handleCancelGesture()
		return true
	}
	if action == cancelRun && a.queueEditor.closable() {
		a.runs.cancel()
		return true
	}
	if a.stack.hasModal() {
		return a.handleGlobalAction(action)
	}
	if a.handleGlobalAction(action) {
		return true
	}
	if a.handleSessionAction(action) {
		return true
	}
	// Completion owns its navigation and acceptance keys before prompt-level
	// history, submission, or editing actions, matching the visual layer on top.
	if action == sendPrompt && a.completion.exactCommandCompletion() {
		if !a.completion.popup.Dismiss() {
			return true
		}
	} else if a.completion.handleCompletion(event) {
		return true
	}
	if a.shell.PromptFocused() && a.handleHistoryAction(action) {
		return true
	}
	if !a.shell.PromptFocused() {
		return false
	}
	switch action {
	case sendPrompt:
		a.submit()
		return true
	case insertNewline:
		before := a.composer.Controller().Revision()
		if a.composer.Controller().Do(action) {
			a.scheduleDraftPersistenceAfter(before)
			a.completion.refreshCompletion()
			return true
		}
	}
	return false
}

func (a *app) handleUnboundEvent(event input.Event) bool {
	if a.stack.hasModal() {
		return false
	}
	if a.shell.TranscriptFocused() {
		switch a.transcript.action(event) {
		case transcriptPrompt:
			return false
		case commandPalette:
			a.showCommandPalette()
			return true
		case openReader:
			a.OpenReader()
			return true
		}
	}
	if a.completion.handleCompletion(event) {
		return true
	}
	if isEscapeEvent(event) {
		return a.handleEscape()
	}
	if a.shell.TranscriptFocused() && isPromptTextEvent(event) && a.shell.FocusPrompt() {
		return a.composer.Controller().Handle(event)
	}
	return false
}

func (a *app) disarmConfirmation(event input.Event, action keymap.Action) {
	switch event := event.(type) {
	case input.Key:
		if !event.Down() {
			return
		}
		if a.confirmation.Armed(confirmClearDraft) && event.Code == input.Esc {
			return
		}
		if a.confirmation.Armed(confirmQuit) && action == quitApp {
			return
		}
	case input.Paste:
	case input.Mouse:
		if event.Action != input.MouseDown {
			return
		}
	default:
		return
	}
	a.confirmation.Reset()
}

func isEscapeEvent(event input.Event) bool {
	key, ok := event.(input.Key)
	return ok && key.Down() && key.Code == input.Esc
}

func (a *app) handleEscape() bool {
	if a.runs.blocksAdmission() || a.operations.Active(inputPreparationOperation) {
		a.confirmation.Reset()
		a.runs.cancel()
		return true
	}
	message, hasDraft, err := a.composer.draft()
	if err != nil {
		a.message(err.Error())
		return true
	}
	if !hasDraft || !a.shell.PromptFocused() {
		a.confirmation.Reset()
		return false
	}
	if !a.confirmation.Confirm(confirmClearDraft, time.Now()) {
		a.status.note("press Esc again to clear the draft")
		return true
	}
	if err := a.retireDraft(message); err != nil {
		a.message("draft clear blocked: " + err.Error())
		return true
	}
	a.status.note("draft cleared")
	return true
}

func (a *app) handleQuit() {
	if !a.confirmation.Confirm(confirmQuit, time.Now()) {
		a.message("repeat " + formatKeyBindings(a.keys.application, quitApp, " or ") + " to quit")
		return
	}
	a.loop.Quit()
}

func isPromptTextEvent(event input.Event) bool {
	if paste, ok := event.(input.Paste); ok {
		return paste.Text != ""
	}
	key, ok := event.(input.Key)
	if !ok || !key.Down() || key.Mods&^input.Shift != 0 {
		return false
	}
	if key.Text != "" {
		return true
	}
	return key.Code == input.Character && key.Rune != 0
}

func (a *app) handleGlobalAction(action keymap.Action) bool {
	if action != cancelRun {
		return false
	}
	a.handleCancelGesture()
	return true
}

func (a *app) handleCancelGesture() {
	if a.prompts.answering() || a.operations.Active(inputPreparationOperation) {
		a.runs.cancel()
		return
	}
	if a.lifecycle.cancelSessionChange() {
		return
	}
	message, hasDraft, err := a.composer.draft()
	if err != nil {
		a.message(err.Error())
		return
	}
	if hasDraft {
		if err := a.retireDraft(message); err != nil {
			a.message("draft clear blocked: " + err.Error())
			return
		}
		a.message("draft cleared; repeat " + formatKeyBindings(a.keys.application, cancelRun, " or ") + " to cancel")
		return
	}
	a.runs.cancel()
}

func (a *app) retireDraft(message authoring.Message) error {
	if err := a.composer.rememberPrompt(message); err != nil {
		return fmt.Errorf("save prompt history: %w", err)
	}
	if err := a.composer.commitDraft(authoring.Message{}); err != nil {
		return fmt.Errorf("retire session draft: %w", err)
	}
	a.completion.popup.Dismiss()
	return nil
}

func (a *app) handleSessionAction(action keymap.Action) bool {
	switch action {
	case commandPalette:
		a.showCommandPalette()
		return true
	case showShortcuts:
		a.ShowShortcuts()
		return true
	case showSessions:
		a.sessions.ShowSessions()
		return true
	case showTimeline:
		a.ShowTimeline()
		return true
	case chooseModel, editPrompt:
		return a.handlePromptAction(action)
	case searchTranscript:
		a.search.show()
		return true
	case manageQueue:
		a.queueEditor.ShowQueue()
		return true
	case toggleDetails:
		a.ToggleToolDetails()
		return true
	case nextMatch:
		a.search.step(1)
		return true
	case previousMatch:
		a.search.step(-1)
		return true
	case scrollPageUp, scrollPageDown, scrollTop, scrollBottom:
		if a.completion.popup.Open() {
			return false
		}
		return a.transcript.Scroll(action)
	default:
		return false
	}
}

func (a *app) handlePromptAction(action keymap.Action) bool {
	if !a.shell.PromptFocused() {
		return false
	}
	if action == chooseModel {
		a.screens.options.ChooseModel()
		return true
	}
	if err := a.composer.editExternally(); err != nil {
		a.message(err.Error())
	}
	return true
}

func (a *app) handleHistoryAction(action keymap.Action) bool {
	switch action {
	case historyPrevious:
		if !a.composer.recallPrevious() {
			a.composer.Controller().MoveUp()
		}
		return true
	case historyNext:
		if !a.composer.recallNext() {
			a.composer.Controller().MoveDown()
		}
		return true
	default:
		return false
	}
}

// scheduleDraftPersistenceAfter compares Oolong's semantic content token around
// an event routed to the main composer. Navigation, focus and handled no-op edits
// leave the token unchanged, so they cannot keep postponing durable recovery.
func (a *app) scheduleDraftPersistenceAfter(before uint64) {
	if a.composer.Controller().Revision() != before {
		a.composer.scheduleDraftPersistence()
	}
}
