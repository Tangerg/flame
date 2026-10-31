package terminal

import (
	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
)

// sessionState owns the active Runtime projection and the lease that prevents
// work started for an older Session from mutating its replacement.
type sessionState struct {
	current         conversation.Session
	context         *sessionContextLease
	invalidated     bool
	draftTransition *sessionDraftTransition
}

type openingReceipt struct {
	RunID     string
	CommandID authoring.CommandID
}

// executionState owns the local observation lifetime. Opening retains the
// acknowledged command association until projection or local settlement takes
// over; Runtime owns the product lifecycle facts.
type executionState struct {
	conversation     *conversation.Conversation
	opening          *openingReceipt
	pendingCancel    *pendingCancellation
	following        bool
	projectionFailed bool
	stopClock        func()
	clock            activeDurationClock
}

func (e executionState) observing() bool {
	return e.conversation.Busy() || e.following
}

func (e executionState) blocksAdmission() bool {
	return e.observing() || e.pendingCancel != nil
}

func (r *runs) runAdmissionBlocked() bool {
	return r.execution.blocksAdmission() || r.session.invalidated || r.tasks.operations.BlocksRunAdmission()
}

func (r *runs) runtimeChangeBlocksRunAdmission() bool {
	return r.session.invalidated || r.tasks.operations.BlocksRunAdmission()
}

// dialogState owns transient modal presentation. Keeping it separate from the
// application root makes it impossible to mistake dialog drafts, selections,
// or readers for durable Runtime state.
type dialogState struct {
	activeContextEditor *contextEditorSession
}
