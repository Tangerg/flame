package terminal

import "github.com/Tangerg/flame/cli/internal/domain/agent"

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

func (a *app) canPreserveInteractionProjection(next *agent.Conversation) bool {
	return a.dialogs.interactionReview != nil && a.execution.conversation.Phase() == agent.ConversationWaiting &&
		next != nil && next.Phase() == agent.ConversationWaiting && a.execution.conversation.RunID() == next.RunID() &&
		sameInteractions(a.execution.conversation.Interactions(), next.Interactions())
}

func (a *app) prepareSessionProjectionReplacement(next agent.Session, conversation *agent.Conversation) {
	if next.ID != a.session.current.ID || next.Workspace != a.session.current.Workspace {
		a.retireSessionContext()
		return
	}
	if a.dialogs.reader.ObservingSource() {
		a.dismissReader()
	}
	if !a.canPreserveInteractionProjection(conversation) {
		a.dismissInteractionProjection()
	}
}

func (a *app) retireSessionContext() {
	a.session.context.retire()
	a.session.context = newSessionContextLease()
	a.dismissInteractionProjection()
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
