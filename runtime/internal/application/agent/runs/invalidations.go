package runs

import (
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
)

type runPublications struct {
	openings      OpeningCommitter
	events        EventCommitter
	barriers      TreeBarrierCommitter
	workspace     WorkspaceChangeNotifier
	invalidations invalidation.Publish
	now           func() time.Time
}

func newRunPublications(
	projection PublicationPorts,
	invalidations invalidation.Publish,
	now func() time.Time,
) runPublications {
	return runPublications{
		openings:      projection.Openings,
		events:        projection.Events,
		barriers:      projection.Barriers,
		workspace:     projection.Workspace,
		invalidations: invalidations,
		now:           now,
	}
}

func (r *runPublications) nowUTC() time.Time { return r.now().UTC() }

func (r *runPublications) event(runID identity.RunID, segmentID identity.SegmentID, reduced reduction) Event {
	return Event{
		RunID: runID, SegmentID: segmentID,
		Timestamp: r.nowUTC(), Payload: reduced.Event,
	}
}

// Published after the durable transition they describe: a client told a run moved
// and then reading the old record would be worse than not being told.

func (r *runPublications) publishRunMoved(sessionID identity.SessionID, runID identity.RunID) {
	r.invalidations.Notify(
		invalidation.InSession(invalidation.Runs, sessionID, runID),
		invalidation.InSession(invalidation.Sessions, sessionID),
	)
}

func (r *runPublications) publishWaitingMoved(sessionID identity.SessionID, runID identity.RunID) {
	r.invalidations.Notify(
		invalidation.InSession(invalidation.Runs, sessionID, runID),
		invalidation.InSession(invalidation.Interrupts, sessionID, runID),
		invalidation.InSession(invalidation.Sessions, sessionID),
	)
}

func (r *runPublications) publishWaitingSubtreeCanceled(
	sessionID identity.SessionID,
	rootRunID identity.RunID,
	affectedRunIDs []identity.RunID,
) {
	r.invalidations.Notify(
		invalidation.InSession(invalidation.Runs, sessionID, affectedRunIDs...),
		invalidation.InSession(invalidation.Interrupts, sessionID, rootRunID),
		invalidation.InSession(invalidation.Sessions, sessionID),
	)
}

func (r *runPublications) publishApprovalsChanged() {
	r.invalidations.Notify(invalidation.Notice{Resource: invalidation.Approvals})
}

func (r *runPublications) publishGoalMoved(sessionID identity.SessionID) {
	r.invalidations.Notify(invalidation.InSession(invalidation.Goals, sessionID))
}
