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
	conversation  *conversation.Conversation
	opening       *openingReceipt
	pendingCancel *pendingCancellation
	observation   observation
	stopClock     func()
	clock         activeDurationClock
}

func (e executionState) observing() bool {
	return e.conversation.Busy() || e.following()
}

// observation is how this terminal watches the current Run: not at all,
// following its event stream, or holding a projection it no longer trusts
// after a local failure while the Run may still be running.
type observation uint8

const (
	observationDetached observation = iota
	observationFollowing
	observationFailed
)

func (e executionState) following() bool { return e.observation == observationFollowing }

func (e executionState) projectionFailed() bool { return e.observation == observationFailed }

// stopFollowing ends the stream without forgetting a failed projection.
func (e *executionState) stopFollowing() {
	if e.observation == observationFollowing {
		e.observation = observationDetached
	}
}

// trustProjection forgets a local failure once a fresh projection replaces it.
func (e *executionState) trustProjection() {
	if e.observation == observationFailed {
		e.observation = observationDetached
	}
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
