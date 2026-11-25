package runs

import (
	"context"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
)

type Nudge struct {
	WorkspaceCWD string
	Paths        []string
}

type OpeningCommitter interface {
	CommitOpening(ctx context.Context, opening OpeningCommit) error
}

type EventCommitter interface {
	CommitExecutionTree(ctx context.Context, commit ExecutionTreeCommit) error
	CommitEvent(ctx context.Context, commit EventCommit) error
}

type TreeBarrierCommitter interface {
	CommitTreeBarrier(ctx context.Context, barrier TreeBarrierCommit) error
}

type WaitingCheckpointReader interface {
	ReadWaitingCheckpoint(ctx context.Context, rootMemberID string) (ExecutorCheckpoint, error)
}

type WaitingSubtreeCancellationCommitter interface {
	CommitWaitingSubtreeCancellation(
		ctx context.Context,
		commit WaitingSubtreeCancellationCommit,
		receipt CancellationReceipt,
	) (WaitingSubtreeCancellationResult, error)
}

type WorkspaceChangeNotifier interface {
	Nudge(cwd string, paths []string)
}

type SegmentFinalizer interface {
	Finish(ctx context.Context, fin Finish) error
}

type Finish struct {
	SessionID       identity.SessionID
	RunID           identity.RunID
	WorkspaceCWD    string
	OpeningUserText string
	ending          run.Ending
}

func (f Finish) Parked() bool { return f.ending == nil }

func (f Finish) Outcome() (run.Outcome, bool) {
	if f.Parked() {
		return "", false
	}
	return f.ending.Outcome(), true
}
