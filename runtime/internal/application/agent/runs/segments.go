package runs

import (
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/application/ownership"
	"github.com/Tangerg/flame/runtime/internal/application/taskgroup"
	"github.com/samber/lo"
)

// Segments supervises live Run segments: it opens them, pumps their execution
// facts through each segment's journal, addresses subscribers and steers to
// the live owner, and keeps segments alive across client disconnects until
// shutdown cancels and joins them. The use cases decide when a segment opens;
// only this owner runs one.
type Segments struct {
	registry     registry
	tasks        taskgroup.Group
	epoch        replayEpoch
	retention    Retention
	observations ExecutionObserver
	releases     ExecutionReleaser
	finalizer    SegmentFinalizer
	childStarts  ChildRunStartCommitter
	runs         Projection
	admission    *ownership.Gate
	publications runPublications
	newRunID     func() string
	newSegmentID func() string
}

// PublicationPorts commit what a segment publishes, in durable-write-before-
// notify order.
type PublicationPorts struct {
	Openings  OpeningCommitter
	Events    EventCommitter
	Barriers  TreeBarrierCommitter
	Workspace WorkspaceChangeNotifier
}

// SegmentDependencies is the complete collaborator set of the segment supervisor.
type SegmentDependencies struct {
	Observations ExecutionObserver
	Releases     ExecutionReleaser
	Publication  PublicationPorts
	ChildStarts  ChildRunStartCommitter
	Finalizer    SegmentFinalizer
	Runs         Projection
	Admissions   *ownership.Gate
	Now          func() time.Time
	// Retention bounds every segment's replay window; discovery advertises it.
	Retention    Retention
	NewRunID     func() string
	NewSegmentID func() string
	// Invalidations publishes run/session/interrupt/state invalidations for clients that
	// are not following the run. nil disables them (no runtime change stream wired).
	Invalidations invalidation.Publish
}

func NewSegments(deps SegmentDependencies) (*Segments, error) {
	for _, required := range []struct {
		name  string
		value any
	}{
		{"execution observer", deps.Observations},
		{"execution releaser", deps.Releases},
		{"opening committer", deps.Publication.Openings},
		{"event committer", deps.Publication.Events},
		{"tree barrier committer", deps.Publication.Barriers},
		{"workspace change notifier", deps.Publication.Workspace},
		{"child run start committer", deps.ChildStarts},
		{"segment finalizer", deps.Finalizer},
		{"run projection", deps.Runs},
		{"admission gate", deps.Admissions},
		{"clock", deps.Now},
		{"run id generator", deps.NewRunID},
		{"segment id generator", deps.NewSegmentID},
	} {
		if lo.IsNil(required.value) {
			return nil, fmt.Errorf("runs: %s is required", required.name)
		}
	}
	if deps.Retention.MaxEvents <= 0 || deps.Retention.MaxBytes <= 0 {
		return nil, errors.New("runs: replay retention budgets must be positive")
	}
	return &Segments{
		observations: deps.Observations,
		releases:     deps.Releases,
		finalizer:    deps.Finalizer,
		childStarts:  deps.ChildStarts,
		retention:    deps.Retention,
		epoch:        newReplayEpoch(),
		runs:         deps.Runs,
		admission:    deps.Admissions,
		publications: newRunPublications(deps.Publication, deps.Invalidations, deps.Now),
		newRunID:     deps.NewRunID,
		newSegmentID: deps.NewSegmentID,
	}, nil
}
