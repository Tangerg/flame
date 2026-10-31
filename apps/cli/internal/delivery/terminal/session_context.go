package terminal

import (
	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
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
	return a.prompts.review != nil && a.runs.conversation().Phase() == conversation.Waiting &&
		next != nil && next.Phase() == conversation.Waiting && a.runs.conversation().RunID() == next.RunID() &&
		sameInterrupts(a.runs.conversation().Interrupts(), next.Interrupts())
}

func (a *app) prepareSessionProjectionReplacement(next conversation.Session, projection *conversation.Conversation) {
	if next.ID != a.session.current.ID || next.Workspace != a.session.current.Workspace {
		a.retireSessionContext()
		return
	}
	if a.reader.observingSource() {
		a.reader.dismiss()
	}
	if !a.canPreserveInterruptProjection(projection) {
		a.prompts.dismiss()
	}
}

func (a *app) retireSessionContext() {
	a.session.context.retire()
	a.session.context = newSessionContextLease()
	a.prompts.dismiss()
	a.confirm.dismiss()
	a.reader.dismiss()
	a.dismissContextEditor()
	a.search.dialog.Dismiss()
	a.palette.dialog.Dismiss()
	a.timeline.dialog.Dismiss()
	a.workspaceChoices.dialog.Dismiss()
	a.dismissModelPicker()
	a.dialogs.sessionDialog.Dismiss()
	a.queueEditor.dialog.Dismiss()
	if a.dialogs.sessionRenameDialog != nil {
		a.dialogs.sessionRenameDialog.Controller().Dismiss()
		a.dialogs.sessionRenameDialog = nil
	}
	if a.dialogs.sessionDeleteDialog != nil {
		a.dialogs.sessionDeleteDialog.Controller().Dismiss()
		a.dialogs.sessionDeleteDialog = nil
	}
}
