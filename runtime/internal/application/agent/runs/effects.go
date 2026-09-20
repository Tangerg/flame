package runs

import (
	"context"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
)

// Nudge is a non-durable live workspace change notification.
type Nudge struct {
	WorkspaceCWD string
	Paths        []string
}

// OpeningCommitter persists one fresh admission or continuation before the
// corresponding Run events are published.
type OpeningCommitter interface {
	CommitOpening(ctx context.Context, opening OpeningCommit) error
}

// ResumeClaimCommitter atomically consumes one complete waiting hand-off and
// invalidates its prior executor checkpoint, returning the claimed snapshot only
// to the active continuation use case.
type ResumeClaimCommitter interface {
	ClaimResume(ctx context.Context, claim ResumeClaimCommit) (ClaimedResume, error)
}

// EventCommitter persists reduced facts and verifies their durable publication
// receipts under the currently active Segment's ownership.
type EventCommitter interface {
	CommitExecutionTree(ctx context.Context, update ExecutionTreeUpdate, commits []EventCommit) error
	CommitEvent(ctx context.Context, commit EventCommit) error
}

// TreeBarrierCommitter atomically persists a complete waiting tree boundary.
type TreeBarrierCommitter interface {
	CommitTreeBarrier(ctx context.Context, barrier TreeBarrierCommit) error
}

// WaitingCheckpointReader returns one exact opaque waiting-tree recovery point.
type WaitingCheckpointReader interface {
	ReadWaitingCheckpoint(ctx context.Context, rootMemberID string) (run.Checkpoint, error)
}

// WaitingSubtreeCancellationCommitter owns the atomic application write-set for
// canceling a child subtree while its root is waiting.
type WaitingSubtreeCancellationCommitter interface {
	CommitWaitingSubtreeCancellation(
		ctx context.Context,
		commit WaitingSubtreeCancellationCommit,
	) (WaitingSubtreeCancellationResult, error)
}

// WorkspaceChangeNotifier emits a best-effort live workspace invalidation after
// the durable transcript projection has committed.
type WorkspaceChangeNotifier interface {
	Nudge(cwd string, paths []string)
}

// SegmentFinalizer performs post-boundary maintenance after the live stream has
// closed. It cannot change the already committed Run outcome.
type SegmentFinalizer interface {
	Finish(ctx context.Context, fin Finish) error
}

// ProjectionPorts groups independently consumed Run projection ports only at
// composition. Construction distributes them into the publication, waiting and
// child-opening behavior owners; no runtime object retains this bundle.
type ProjectionPorts struct {
	Openings                    OpeningCommitter
	ChildStarts                 ChildRunStartCommitter
	ResumeClaims                ResumeClaimCommitter
	Events                      EventCommitter
	Barriers                    TreeBarrierCommitter
	Checkpoints                 WaitingCheckpointReader
	WaitingSubtreeCancellations WaitingSubtreeCancellationCommitter
	Workspace                   WorkspaceChangeNotifier
	Finalizer                   SegmentFinalizer
}

// Finish describes maintenance after a segment closes, including a parked Run.
type Finish struct {
	SessionID       string
	RunID           string
	WorkspaceCWD    string
	Parked          bool
	OpeningUserText string
}
