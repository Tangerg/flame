package sessions

import (
	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
)

func (c *Coordinator) publishSessionMoved(sessionID identity.SessionID) {
	c.invalidations.Notify(invalidation.InSession(invalidation.Sessions, sessionID))
}

func (c *Coordinator) publishRunsMoved(sessionID identity.SessionID, copied []run.Run) {
	runIDs := make([]identity.RunID, len(copied))
	for index, value := range copied {
		runIDs[index] = value.ID()
	}
	c.invalidations.Notify(invalidation.Notice{
		Resource: invalidation.Runs, SessionIDs: []identity.SessionID{sessionID}, RunIDs: runIDs,
	})
}

func (c *Coordinator) publishPlanMoved(sessionIDs ...identity.SessionID) {
	c.invalidations.Notify(invalidation.InSessions(invalidation.Plan, sessionIDs...))
}

func (c *Coordinator) publishAggregateMoved(sessionIDs []identity.SessionID, runIDs []identity.RunID) {
	c.invalidations.Notify(
		invalidation.InSessions(invalidation.Sessions, sessionIDs...),
		invalidation.Notice{Resource: invalidation.Runs, SessionIDs: sessionIDs, RunIDs: runIDs},
		invalidation.InSessions(invalidation.Interrupts, sessionIDs...),
		invalidation.InSessions(invalidation.Goals, sessionIDs...),
		invalidation.InSessions(invalidation.Plan, sessionIDs...),
	)
}
