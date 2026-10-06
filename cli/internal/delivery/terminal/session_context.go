package terminal

import (
	"github.com/Tangerg/flame/cli/internal/domain/conversation"
)

type sessionContextLease struct {
	retired bool
}

func newSessionContextLease() *sessionContextLease {
	return &sessionContextLease{}
}

func (s *sessionContextLease) retire() {
	if s != nil {
		s.retired = true
	}
}

func (s *sessionContextLease) current(candidate *sessionContextLease) bool {
	return s != nil && s == candidate && !s.retired
}

func (a *app) canPreserveInterruptProjection(next *conversation.Conversation) bool {
	return a.dialogs.interruptReview != nil && a.execution.conversation.Phase() == conversation.Waiting &&
		next != nil && next.Phase() == conversation.Waiting && a.execution.conversation.RunID() == next.RunID() &&
		sameInterrupts(a.execution.conversation.Interrupts(), next.Interrupts())
}

func (a *app) prepareSessionProjectionReplacement(next conversation.Session, projection *conversation.Conversation) {
	if next.ID != a.session.current.ID || next.Workspace != a.session.current.Workspace {
		a.retireSessionContext()
		return
	}
	if a.dialogs.reader.ObservingSource() {
		a.dismissReader()
	}
	if !a.canPreserveInterruptProjection(projection) {
		a.dismissInterruptProjection()
	}
}

func (a *app) retireSessionContext() {
	a.session.context.retire()
	a.session.context = newSessionContextLease()
	a.dismissInterruptProjection()
	a.dismissConfirmation()
	a.dismissReader()
	a.dismissContextEditor()
	a.dialogs.searchDialog.Dismiss()
	a.dialogs.commandDialog.Dismiss()
	a.dialogs.timelineDialog.Dismiss()
	a.dialogs.workspaceDialog.Dismiss()
	a.dialogs.modelDialog.Dismiss()
	a.dialogs.sessionDialog.Dismiss()
	a.dialogs.queueDialog.Dismiss()
	if a.dialogs.sessionRenameDialog != nil {
		a.dialogs.sessionRenameDialog.Controller().Dismiss()
		a.dialogs.sessionRenameDialog = nil
	}
	if a.dialogs.sessionDeleteDialog != nil {
		a.dialogs.sessionDeleteDialog.Controller().Dismiss()
		a.dialogs.sessionDeleteDialog = nil
	}
	if a.dialogs.scheduleDialog != nil {
		a.dialogs.scheduleDialog.Controller().Dismiss()
		a.dialogs.scheduleDialog = nil
	}
}
