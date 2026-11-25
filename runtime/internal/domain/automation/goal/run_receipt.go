package goal

import (
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/goalworkflow"
	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/packages/go-sdk/runoutcome"
	"github.com/Tangerg/flame/runtime/internal/domain/automation/goalref"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
)

type RunBoundary interface {
	goalRequest(Goal) (goalworkflow.Request, bool)
}

type RunPark struct {
	SegmentID identity.SegmentID
}

func (RunPark) goalRequest(current Goal) (goalworkflow.Request, bool) {
	return goalworkflow.Request{Signal: goalworkflow.Pause, Reason: goalworkflow.GoalReasonAwaitingInput}, current.Status() == StatusActive
}

type RunEnd struct {
	Outcome run.Outcome
}

func (e RunEnd) goalRequest(Goal) (goalworkflow.Request, bool) {
	return goalworkflow.Request{Signal: goalworkflow.TerminalRun, Outcome: runoutcome.Kind(e.Outcome)}, true
}

type RunReceipt struct {
	RunID         identity.RunID
	SessionID     identity.SessionID
	IncarnationID goalref.IncarnationID
	Revision      uint64
	Boundary      RunBoundary
	OccurredAt    time.Time
}

func (r RunReceipt) Owns(version Version) bool {
	incarnation, committed := version.IncarnationID()
	revision, _ := version.Revision()
	return committed && r.SessionID == version.SessionID() && r.IncarnationID == incarnation && r.Revision == revision
}

func (r RunReceipt) RequestFor(current Goal) (goalworkflow.Request, bool) {
	if !r.Owns(current.Version()) {
		return goalworkflow.Request{}, false
	}
	return r.Boundary.goalRequest(current)
}
