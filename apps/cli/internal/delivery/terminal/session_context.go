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

func (l *sessionLifecycle) canPreserveInterruptProjection(next *conversation.Conversation) bool {
	return l.prompts.review != nil && l.runs.conversation().Phase() == conversation.Waiting &&
		next != nil && next.Phase() == conversation.Waiting && l.runs.conversation().RunID() == next.RunID() &&
		sameInterrupts(l.runs.conversation().Interrupts(), next.Interrupts())
}

func (l *sessionLifecycle) prepareSessionProjectionReplacement(next conversation.Session, projection *conversation.Conversation) {
	if next.ID != l.state.current.ID || next.Workspace != l.state.current.Workspace {
		l.retireSessionContext()
		return
	}
	if l.reader.observingSource() {
		l.reader.dismiss()
	}
	if !l.canPreserveInterruptProjection(projection) {
		l.prompts.dismiss()
	}
}

func (l *sessionLifecycle) retireSessionContext() {
	l.state.context.retire()
	l.state.context = newSessionContextLease()
	l.root.dismissModals()
}

// dismissModals closes every modal that belonged to the retired Session.
func (a *app) dismissModals() {
	a.prompts.dismiss()
	a.confirm.dismiss()
	a.reader.dismiss()
	a.dismissContextEditor()
	a.search.dialog.Dismiss()
	a.palette.dialog.Dismiss()
	a.timeline.dialog.Dismiss()
	a.workspaceChoices.dialog.Dismiss()
	a.screens.options.dismissModelPicker()
	a.sessions.dismiss()
	a.queueEditor.dialog.Dismiss()
}
