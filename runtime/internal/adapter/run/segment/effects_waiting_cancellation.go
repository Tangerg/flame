package segment

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
)

// CommitWaitingSubtreeCancellation claims the prepared Pending snapshot and
// persists the application-defined replacement atomically. It does not decide
// which Runs survive or how their transcript and continuation facts change.
func (e *Effects) CommitWaitingSubtreeCancellation(
	ctx context.Context,
	commit runs.WaitingSubtreeCancellationCommit,
) (runs.WaitingSubtreeCancellationResult, error) {
	if err := commit.Validate(); err != nil {
		return runs.WaitingSubtreeCancellationResult{}, fmt.Errorf(
			"segment: invalid waiting subtree cancellation: %w",
			err,
		)
	}
	var target, root run.Run
	err := e.runInTx(ctx, func(ctx context.Context) error {
		if err := e.claimWaitingCancellation(ctx, commit); err != nil {
			return err
		}
		if err := e.persistWaitingCancellationProjection(ctx, commit); err != nil {
			return err
		}
		terminalByID, err := e.terminalizeWaitingCancellationRuns(ctx, commit.TerminalRuns())
		if err != nil {
			return err
		}
		var targetFound bool
		target, targetFound = terminalByID[commit.TargetRunID()]
		if !targetFound {
			return fmt.Errorf("segment: target Run %q was not terminalized", commit.TargetRunID())
		}

		root = commit.RootRun()
		return e.persistWaitingCancellationDisposition(ctx, commit, &root)
	})
	if err != nil {
		settled, committed, settleErr := e.reconcileWaitingCancellation(ctx, commit)
		if settled {
			return committed, nil
		}
		if settleErr != nil {
			err = errors.Join(err, settleErr)
		}
		return runs.WaitingSubtreeCancellationResult{}, fmt.Errorf(
			"segment: commit waiting child Run %q cancellation in root Run %q: %w",
			commit.TargetRunID(),
			commit.RootRunID(),
			err,
		)
	}
	return runs.WaitingSubtreeCancellationResult{TargetRun: target, RootRun: root}, nil
}

func (e *Effects) claimWaitingCancellation(
	ctx context.Context,
	commit runs.WaitingSubtreeCancellationCommit,
) error {
	pending, found, err := e.interrupts.Consume(ctx, commit.SessionID(), commit.RootRunID())
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
			runs.ErrSessionBusy,
			commit.RootRunID(),
		)
	}
	if !pending.Equal(commit.ExpectedPending()) {
		return fmt.Errorf(
			"%w: waiting cancellation interrupt for root Run %q changed after preparation",
			runs.ErrSessionBusy,
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
) (map[string]run.Run, error) {
	terminalByID := make(map[string]run.Run, len(planned))
	for _, replacement := range planned {
		runRecord := replacement.State()
		finalized, err := e.finishedRun(ctx, runRecord)
		if err != nil {
			return nil, fmt.Errorf("segment: finalize canceled Run %q: %w", runRecord.ID(), err)
		}
		finalReplacement, err := run.NewReplacement(replacement.Expected(), finalized)
		if err != nil {
			return nil, fmt.Errorf("segment: finalize canceled Run %q replacement: %w", runRecord.ID(), err)
		}
		if err := e.runState.Terminalize(ctx, finalReplacement); err != nil {
			return nil, fmt.Errorf("segment: terminalize canceled Run %q: %w", runRecord.ID(), err)
		}
		terminalByID[runRecord.ID()] = finalized
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
	for _, draft := range resume.Runs {
		if err := e.runState.Resume(ctx, resume.SessionID, draft, resume.ResumedAt); err != nil {
			return fmt.Errorf("segment: resume surviving Run %q: %w", draft.RunID, err)
		}
		if draft.RunID == commit.RootRunID() {
			resumed, err := root.Resume(draft.SegmentID, resume.ResumedAt)
			if err != nil {
				return fmt.Errorf("segment: project resumed root Run %q: %w", draft.RunID, err)
			}
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

func waitingCancellationRootSegmentID(commit runs.WaitingSubtreeCancellationCommit) (string, error) {
	if _, stillWaiting := commit.RemainingPending(); stillWaiting {
		return "", nil
	}
	resume, resuming := commit.Resume()
	if !resuming {
		return "", errors.New("segment: waiting cancellation has no surviving disposition")
	}
	for _, draft := range resume.Runs {
		if draft.RunID == commit.RootRunID() {
			return draft.SegmentID, nil
		}
	}
	return "", errors.New("segment: waiting cancellation resume has no root Run")
}

func (e *Effects) reconcileWaitingCancellation(
	ctx context.Context,
	commit runs.WaitingSubtreeCancellationCommit,
) (bool, runs.WaitingSubtreeCancellationResult, error) {
	segmentID, err := waitingCancellationRootSegmentID(commit)
	if err != nil {
		return false, runs.WaitingSubtreeCancellationResult{}, err
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
		return false, runs.WaitingSubtreeCancellationResult{}, fmt.Errorf(
			"segment: reconcile waiting cancellation commit: %w",
			err,
		)
	}
	if !settled {
		return false, runs.WaitingSubtreeCancellationResult{}, nil
	}
	target, err := e.reconciledRun(reconcileCtx, "target", commit.TargetRunID(), commit.SessionID())
	if err != nil {
		return false, runs.WaitingSubtreeCancellationResult{}, err
	}
	root, err := e.reconciledRun(reconcileCtx, "root", commit.RootRunID(), commit.SessionID())
	if err != nil {
		return false, runs.WaitingSubtreeCancellationResult{}, err
	}
	return true, runs.WaitingSubtreeCancellationResult{TargetRun: target, RootRun: root}, nil
}

// reconciledRun reads back one Run a settled cancellation commit named. A Run
// that is gone and a Run that belongs to another Session are the same answer
// here: the commit this reconciles did not produce it.
func (e *Effects) reconciledRun(ctx context.Context, kind, runID, sessionID string) (run.Run, error) {
	value, found, err := e.runState.Run(ctx, runID)
	if err != nil {
		return run.Run{}, fmt.Errorf("segment: read reconciled %s Run %q: %w", kind, runID, err)
	}
	if !found || value.SessionID() != sessionID {
		return run.Run{}, fmt.Errorf("segment: reconciled %s Run %q is unavailable", kind, runID)
	}
	return value, nil
}
