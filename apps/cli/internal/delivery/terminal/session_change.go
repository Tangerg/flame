package terminal

import (
	"context"
	"errors"

	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
)

var errSessionRequiredForPrompt = errors.New("open a session before sending a prompt")

type sessionChange struct {
	baseline authoring.Message
	receipt  *sessionChangeReceipt
}

type sessionChangeText struct {
	working   string
	committed string
}

type sessionChangeReceipt struct {
	session conversation.Session
	action  string
}

func (r sessionChangeReceipt) summary() string {
	return r.action + " · " + displayTitle(r.session)
}

type sessionOpeningSource uint8

const (
	readCommittedSession sessionOpeningSource = iota
	openCreatedSession
)

func (l *sessionLifecycle) runSessionMutation(text sessionChangeText, source sessionOpeningSource, command func(context.Context) (conversation.Session, error)) {
	if !l.beginSessionChange(text.working) {
		return
	}
	l.runSessionChangeStep(text.working, command, func(committed conversation.Session, err error) {
		if err != nil {
			l.root.message(text.working + " failed: " + err.Error())
			l.settleSessionChange()
			return
		}
		receipt := &sessionChangeReceipt{session: committed, action: text.committed}
		l.state.change.receipt = receipt
		if source == openCreatedSession {
			l.finishSessionOpening(conversation.SessionSnapshot{Session: committed}, nil)
			return
		}
		l.runSessionChangeStep(receipt.summary()+" · opening",
			func(ctx context.Context) (conversation.SessionSnapshot, error) {
				return l.readSessionAfterMutation(ctx, receipt.session.ID)
			},
			l.finishSessionOpening,
		)
	})
}

func (l *sessionLifecycle) finishSessionOpening(snapshot conversation.SessionSnapshot, err error) {
	if err == nil {
		err = l.installSnapshot(snapshot)
	}
	if err != nil {
		l.abandonSessionOpening("could not open: " + err.Error())
	}
	l.settleSessionChange()
}

func (l *sessionLifecycle) runSessionChange[T any](label string, work func(context.Context) (T, error), apply func(T) error) {
	if !l.beginSessionChange(label) {
		return
	}
	l.runSessionChangeStep(label, work, func(result T, err error) {
		if err == nil {
			err = apply(result)
		}
		if err != nil {
			l.root.message(label + " failed: " + err.Error())
		}
		l.settleSessionChange()
	})
}

func (l *sessionLifecycle) beginSessionChange(label string) bool {
	if l.runs.observing() {
		l.root.message("finish or cancel the current run before changing sessions")
		return false
	}
	if l.runs.cancelPending() {
		l.root.message("wait for runtime cancellation to finish")
		return false
	}
	if l.tasks.operations.Active(sessionChangeOperation) {
		l.root.message(sessionChangeOperation.busy().Error())
		return false
	}
	if l.tasks.operations.Active(sessionCenterOperation) {
		l.root.message(sessionCenterOperation.busy().Error())
		return false
	}
	l.tasks.operations.Cancel(pickerCatalogOperation)
	l.sessions.dialog.Dismiss()
	baseline, _, err := l.composer.draft()
	if err != nil {
		l.root.message(label + " failed: " + err.Error())
		return false
	}
	if err := l.composer.saveDraft(baseline); err != nil {
		l.report(workbenchDraft, err)
		l.root.message("session change blocked: save session draft: " + err.Error())
		return false
	}
	l.report(workbenchDraft, nil)
	if l.tasks.operations.Cancel(sessionInvalidationOperation) {
		l.state.invalidate()
	}
	l.state.change = &sessionChange{baseline: baseline}
	return true
}

func (l *sessionLifecycle) runSessionChangeStep[T any](label string, work func(context.Context) (T, error), apply func(T, error)) {
	l.root.message(label)
	if !l.tasks.sessionFence(sessionChangeOperation, work, apply) {
		var zero T
		apply(zero, sessionChangeOperation.busy())
	}
}

func (l *sessionLifecycle) cancelSessionChange() bool {
	if !l.tasks.operations.Cancel(sessionChangeOperation) {
		return false
	}
	if l.state.change.receipt == nil {
		l.root.message("stopped waiting for session change")
	} else {
		l.abandonSessionOpening("opening canceled")
	}
	l.settleSessionChange()
	return true
}

func (l *sessionLifecycle) abandonSessionOpening(reason string) {
	receipt := l.state.change.receipt
	if receipt.session.ID == l.state.current.ID {
		l.state.invalidate()
	}
	l.root.message(receipt.summary() + " · " + reason)
}

func (l *sessionLifecycle) settleSessionChange() {
	l.state.change = nil
	if l.state.freshness == sessionStale && l.runs.conversation().Phase() != conversation.Running &&
		!l.runs.following() && !l.runs.cancelPending() {
		l.refreshInvalidatedSession(refreshAfterChange)
	}
}
