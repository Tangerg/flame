package segment

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run"

	framehost "github.com/Tangerg/frame/host"
)

func (e *Effects) CommitWaitingSubtreeCancellation(
	ctx context.Context,
	commit runs.WaitingSubtreeCancellationCommit,
	receipt runs.CancellationReceipt,
) (runs.WaitingSubtreeCancellationResult, error) {
	var result runs.WaitingSubtreeCancellationResult
	var projected bool
	checkpoint := commit.Checkpoint()
	err := e.runCheckpointTx(ctx, commit.CommitID().String(), &checkpoint, func(ctx context.Context) error {
		if err := e.claimWaitingCancellation(ctx, commit); err != nil {
			return err
		}
		if err := e.persistWaitingCancellationProjection(ctx, commit); err != nil {
			return err
		}
		terminalByID, err := e.terminalizeWaitingCancellationRuns(ctx, commit.TerminalRuns(), commit.TargetRunID(), runs.CancellationKey(receipt))
		if err != nil {
			return err
		}
		target, targetFound := terminalByID[commit.TargetRunID()]
		if !targetFound {
			return fmt.Errorf("segment: target Run %q was not terminalized", commit.TargetRunID())
		}

		root := commit.RootRun()
		if err := e.persistWaitingCancellationDisposition(ctx, commit, &root); err != nil {
			return err
		}
		accepted := commit.AcceptedCancellation()
		if err := e.acceptedContinuations.PutSubtreeCancellation(ctx, accepted); err != nil {
			return err
		}
		result = runs.WaitingSubtreeCancellationResult{TargetRun: target, RootRun: root}
		ref := runs.ExecutorRef{SessionID: accepted.SessionID, ExecutorID: accepted.Pending.ExecutorID}
		if _, err := runs.StageCancellation(ctx, receipt, ref, func(context.Context) (runs.CancelResult, error) {
			return runs.CancelResult{Run: target, RootRun: &root}, nil
		}); err != nil {
			return err
		}
		projected = true
		return nil
	})
	if errors.Is(err, framehost.ErrRecoveryRequired) {
		return runs.WaitingSubtreeCancellationResult{}, err
	}
	if err != nil {
		if projected {
			settled, settleErr := e.reconcileWaitingCancellation(ctx, commit)
			if settled {
				return result, nil
			}
			err = errors.Join(err, settleErr)
		}
		return runs.WaitingSubtreeCancellationResult{}, fmt.Errorf(
			"segment: commit waiting child Run %q cancellation in root Run %q: %w",
			commit.TargetRunID(),
			commit.RootRunID(),
			err,
		)
	}
	return result, nil
}

func (e *Effects) claimWaitingCancellation(
	ctx context.Context,
	commit runs.WaitingSubtreeCancellationCommit,
) error {
	pending, found, err := e.interrupts.Consume(ctx, commit.RootRunID())
	if err != nil {
		return fmt.Errorf(
			"segment: claim waiting cancellation interrupt for root Run %q: %w",
			commit.RootRunID(),
			err,
		)
	}
	if !found {
		return fmt.Errorf(
			"%w: waiting cancellation interrupt for root Run %q is no longer open",
			run.ErrSessionBusy,
			commit.RootRunID(),
		)
	}
	if !pending.Equal(commit.ExpectedPending()) {
		return fmt.Errorf(
			"%w: waiting cancellation interrupt for root Run %q changed after preparation",
			run.ErrSessionBusy,
			commit.RootRunID(),
		)
	}
	return nil
}

func (e *Effects) persistWaitingCancellationProjection(
	ctx context.Context,
	commit runs.WaitingSubtreeCancellationCommit,
) error {
	if err := e.executorCheckpoints.SaveCheckpoint(ctx, commit.Checkpoint()); err != nil {
		return fmt.Errorf(
			"segment: persist checkpoint for waiting child Run %q in root Run %q: %w",
			commit.TargetRunID(),
			commit.RootRunID(),
			err,
		)
	}
	for _, item := range commit.TerminalItems() {
		if err := e.itemReplacer.ReplaceItem(ctx, item); err != nil {
			return fmt.Errorf(
				"segment: settle interrupted Item %q for canceled Run %q: %w",
				item.Expected().ID(),
				item.Expected().RunID(),
				err,
			)
		}
	}
	return nil
}

func (e *Effects) terminalizeWaitingCancellationRuns(
	ctx context.Context,
	planned []run.Replacement,
	targetRunID identity.RunID, winningCancellationKey string,
) (map[identity.RunID]run.Run, error) {
	terminalByID := make(map[identity.RunID]run.Run, len(planned))
	for _, replacement := range planned {
		runID := replacement.State().ID()
		finalReplacement := replacement
		mark, needed, err := e.terminalMessageMark(ctx, replacement.State())
		if err == nil && needed {
			finalReplacement, err = replacement.WithMessageMark(mark)
		}
		if err != nil {
			return nil, fmt.Errorf("segment: finalize canceled Run %q: %w", runID, err)
		}
		key := ""
		if runID == targetRunID {
			key = winningCancellationKey
		}
		if err := e.runState.Terminalize(ctx, finalReplacement, key); err != nil {
			return nil, fmt.Errorf("segment: terminalize canceled Run %q: %w", runID, err)
		}
		terminalByID[runID] = finalReplacement.State()
	}
	return terminalByID, nil
}

func (e *Effects) persistWaitingCancellationDisposition(
	ctx context.Context,
	commit runs.WaitingSubtreeCancellationCommit,
	root *run.Run,
) error {
	if root == nil {
		return errors.New("segment: waiting cancellation root projection is required")
	}
	if remaining, stillWaiting := commit.RemainingPending(); stillWaiting {
		if err := e.interrupts.Open(ctx, remaining); err != nil {
			return fmt.Errorf(
				"segment: persist reduced interrupt for root Run %q: %w",
				commit.RootRunID(),
				err,
			)
		}
		if err := e.runState.RecordWaitingRunCommit(
			ctx, commit.SessionID(), commit.RootRunID(), commit.CommitID(),
		); err != nil {
			return fmt.Errorf("segment: record waiting cancellation commit receipt: %w", err)
		}
		return nil
	}
	resume, resuming := commit.Resume()
	if !resuming {
		return errors.New("segment: waiting cancellation has no surviving disposition")
	}
	for _, replacement := range resume.Runs {
		resumed := replacement.State()
		if err := e.runState.Resume(ctx, replacement); err != nil {
			return fmt.Errorf("segment: resume surviving Run %q: %w", resumed.ID(), err)
		}
		if resumed.ID() == commit.RootRunID() {
			*root = resumed
		}
	}
	for _, event := range commit.OpeningEvents() {
		if err := e.applyCommit(ctx, event); err != nil {
			return fmt.Errorf(
				"segment: persist opening projection for surviving Run %q: %w",
				event.RunID,
				err,
			)
		}
	}
	segmentID, err := waitingCancellationRootSegmentID(commit)
	if err != nil {
		return err
	}
	if err := e.runState.RecordRunCommit(
		ctx, commit.SessionID(), commit.RootRunID(), segmentID, commit.CommitID(),
	); err != nil {
		return fmt.Errorf("segment: record resumed waiting cancellation commit receipt: %w", err)
	}
	return nil
}

func waitingCancellationRootSegmentID(commit runs.WaitingSubtreeCancellationCommit) (identity.SegmentID, error) {
	if _, stillWaiting := commit.RemainingPending(); stillWaiting {
		return identity.SegmentID{}, nil
	}
	resume, resuming := commit.Resume()
	if !resuming {
		return identity.SegmentID{}, errors.New("segment: waiting cancellation has no surviving disposition")
	}
	if resume.RootRunID() != commit.RootRunID() {
		return identity.SegmentID{}, errors.New("segment: waiting cancellation resume has no root Run")
	}
	return resume.Runs[len(resume.Runs)-1].State().ActiveSegmentID(), nil
}

func (e *Effects) reconcileWaitingCancellation(ctx context.Context, commit runs.WaitingSubtreeCancellationCommit) (bool, error) {
	segmentID, err := waitingCancellationRootSegmentID(commit)
	if err != nil {
		return false, err
	}
	reconcileCtx, cancel := context.WithTimeout(
		context.WithoutCancel(ctx),
		runCommitReconciliationTimeout,
	)
	defer cancel()
	settled, err := e.runState.RunCommitCommitted(
		reconcileCtx, commit.SessionID(), commit.RootRunID(), segmentID, commit.CommitID(),
	)
	if err != nil {
		return false, fmt.Errorf(
			"segment: reconcile waiting cancellation commit: %w",
			err,
		)
	}
	return settled, nil
}
