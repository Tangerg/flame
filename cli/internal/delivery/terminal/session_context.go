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
	return a.interactionReview != nil && a.conversation.Phase() == agent.ConversationWaiting &&
		next != nil && next.Phase() == agent.ConversationWaiting && a.conversation.RunID() == next.RunID() &&
		sameInteractions(a.conversation.Interactions(), next.Interactions())
}

func (a *app) prepareSessionProjectionReplacement(next agent.Session, conversation *agent.Conversation) {
	if next.ID != a.session.ID || next.Workspace != a.session.Workspace {
		a.retireSessionContext()
		return
	}
	if a.reader.ObservingSource() {
		a.dismissReader()
	}
	if !a.canPreserveInteractionProjection(conversation) {
		a.dismissInteractionProjection()
	}
}

func (a *app) retireSessionContext() {
	a.sessionContext.retire()
	a.sessionContext = newSessionContextLease()
	a.dismissInteractionProjection()
	a.dismissConfirmation()
	a.dismissReader()
	a.dismissContextEditor()
	a.searchDialog.Dismiss()
	a.commandDialog.Dismiss()
	a.timelineDialog.Dismiss()
	a.workspaceDialog.Dismiss()
	a.modelDialog.Dismiss()
	a.sessionDialog.Dismiss()
	a.queueDialog.Dismiss()
	if a.sessionRenameDialog != nil {
		a.sessionRenameDialog.Controller().Dismiss()
		a.sessionRenameDialog = nil
	}
	if a.sessionDeleteDialog != nil {
		a.sessionDeleteDialog.Controller().Dismiss()
		a.sessionDeleteDialog = nil
	}
	if a.scheduleDialog != nil {
		a.scheduleDialog.Controller().Dismiss()
		a.scheduleDialog = nil
	}
}
