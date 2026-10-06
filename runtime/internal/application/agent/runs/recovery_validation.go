package runs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
)

// validateRecoveryParkedTree checks the complete durable hand-off barrier before
// boot keeps a Run tree resumable. Pending is root-owned, so its continuations
// must cover every non-terminal member exactly once.
//
// An impossible partial application write is corruption and fails startup. A
// missing or executor-incompatible checkpoint is an external-resource loss and
// returns an unresumable verdict naming it, so recovery marks the whole tree
// run_lost with that reason.
func validateRecoveryParkedTree(
	ctx context.Context,
	tree recoveryRunTree,
	pending Pending,
	sess session.Session,
	items []transcript.Item,
	store RecoveryStore,
	resumability WaitingExecutionResumability,
) (WaitingResumption, error) {
	values := make([]run.Run, 0, len(tree.postorder))
	for _, runID := range tree.postorder {
		values = append(values, tree.runsByID[runID])
	}
	if err := pending.ValidateProjection(values, items); err != nil {
		return WaitingResumption{}, fmt.Errorf("runs: validate recovery Run tree %q: %w", tree.root.ID(), err)
	}

	rootContinuation, _ := pending.RootContinuation()
	// Isolated workspaces are process-local scratch copies and are deliberately
	// never snapshotted. A host restart therefore destroys the world this tree
	// was parked in even when its executor payload remains decodable.
	if sess.Isolated() {
		return UnresumableWaiting(LossIsolatedWorkspace), nil
	}
	checkpoint, err := store.LoadExecutorCheckpoint(ctx, rootContinuation.MemberID)
	if errors.Is(err, ErrExecutorCheckpointNotFound) || errors.Is(err, ErrInvalidExecutorCheckpoint) {
		return unresumableRecoveredTree(ctx, tree, rootContinuation.MemberID, err), nil
	}
	if err != nil {
		return WaitingResumption{}, fmt.Errorf(
			"runs: load executor checkpoint %q for recovery: %w",
			rootContinuation.MemberID,
			err,
		)
	}
	if err := checkpoint.ValidateOwnership(rootContinuation.MemberID, tree.root.SessionID()); err != nil {
		return unresumableRecoveredTree(ctx, tree, rootContinuation.MemberID, err), nil
	}
	continuation, err := waitingContinuationFromPending(pending, checkpoint, values, sess)
	if err != nil {
		return WaitingResumption{}, fmt.Errorf(
			"runs: build waiting continuation %q for recovery: %w",
			rootContinuation.MemberID,
			err,
		)
	}
	resumption, err := resumability.CanResumeWaitingExecution(ctx, continuation)
	if err != nil {
		return WaitingResumption{}, fmt.Errorf(
			"runs: probe waiting execution %q resumability: %w",
			rootContinuation.MemberID,
			err,
		)
	}
	return resumption, nil
}

// unresumableRecoveredTree loses a tree whose waiting state cannot be read
// back. The lost Run carries only the Loss; the cause can name paths or payload
// details, so it goes to the operator log, as the executor probe's does.
func unresumableRecoveredTree(ctx context.Context, tree recoveryRunTree, memberID string, cause error) WaitingResumption {
	slog.WarnContext(ctx, "runs: waiting Run tree is not resumable",
		"session.id", tree.root.SessionID(),
		"run.id", tree.root.ID(),
		"executor.member.id", memberID,
		"loss", string(LossWaitingStateUnavailable),
		"error", cause,
	)
	return UnresumableWaiting(LossWaitingStateUnavailable)
}
