package terminal

import (
	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
)

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

type executionState struct {
	conversation  *conversation.Conversation
	opening       *openingReceipt
	pendingCancel *pendingCancellation
	observation   observation
	stopClock     func()
	clock         activeDurationClock
}

func (e *executionState) observing() bool {
	return e.conversation.Busy() || e.following()
}

type observation uint8

const (
	observationDetached observation = iota
	observationFollowing
	observationFailed
)

func (e *executionState) following() bool { return e.observation == observationFollowing }

func (e *executionState) projectionFailed() bool { return e.observation == observationFailed }

func (e *executionState) stopFollowing() {
	if e.observation == observationFollowing {
		e.observation = observationDetached
	}
}

func (e *executionState) trustProjection() {
	if e.observation == observationFailed {
		e.observation = observationDetached
	}
}

func (e *executionState) blocksAdmission() bool {
	return e.observing() || e.pendingCancel != nil
}

func (r *runs) runAdmissionBlocked() bool {
	return r.execution.blocksAdmission() || r.session.invalidated || r.tasks.operations.BlocksRunAdmission()
}

func (r *runs) runtimeChangeBlocksRunAdmission() bool {
	return r.session.invalidated || r.tasks.operations.BlocksRunAdmission()
}
