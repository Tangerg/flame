package runs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/Tangerg/flame/runtime/internal/telemetry"
)

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
	// Isolated workspaces are never snapshotted, so a restart destroys the world the
	// tree was parked in.
	if sess.Isolated() {
		return UnresumableWaiting(run.LossIsolatedWorkspace), nil
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

// The cause can name paths or payload details, so it goes to the operator log
// rather than the Loss.
func unresumableRecoveredTree(ctx context.Context, tree recoveryRunTree, memberID string, cause error) WaitingResumption {
	slog.WarnContext(ctx, "runs: waiting Run tree is not resumable",
		telemetry.SessionID(tree.root.SessionID()),
		telemetry.RunID(tree.root.ID()),
		slog.Any("executor.member.id", memberID),
		telemetry.RunLoss(string(run.LossWaitingStateUnavailable)),
		telemetry.Error(cause),
	)
	return UnresumableWaiting(run.LossWaitingStateUnavailable)
}
