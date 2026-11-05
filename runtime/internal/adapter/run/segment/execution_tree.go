package segment

import (
	"context"
	"errors"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/frame"
)

func (e *Effects) CommitExecutionTree(ctx context.Context, tree runs.ExecutionTreeCommit) (commitErr error) {
	update, commits := tree.Update, tree.Events
	if err := tree.Validate(); err != nil {
		return err
	}
	defer func() {
		if commitErr != nil && !errors.Is(commitErr, frame.ErrRecoveryRequired) {
			for _, commit := range commits {
				commitErr = e.compensateFailedCommit(ctx, commit, commitErr)
			}
		}
	}()
	var applyErr error
	err := e.runCheckpointTx(ctx, update.Head.CommitID, tree.Context, func(ctx context.Context) (err error) {
		defer func() { applyErr = err }()
		return e.applyExecutionTree(ctx, tree)
	})
	if err == nil {
		return nil
	}
	if errors.Is(err, frame.ErrRecoveryRequired) {
		return err
	}
	if applyErr != nil {
		return err
	}
	readCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), stagedToolResultCleanupTimeout)
	defer cancel()
	return e.reconcileExecutionTree(readCtx, tree, err)
}

func (e *Effects) applyExecutionTree(ctx context.Context, tree runs.ExecutionTreeCommit) error {
	if err := e.executionTrees.SaveExecutionTree(ctx, tree.Update); err != nil {
		return err
	}
	for _, acceptance := range tree.InputAcceptances {
		if err := e.commandAcceptances.Save(ctx, acceptance); err != nil {
			return err
		}
	}
	if tree.Context != nil {
		if err := e.executorCheckpoints.SaveCheckpoint(ctx, *tree.Context); err != nil {
			return err
		}
	}
	for _, started := range tree.StartedChildren {
		if err := e.applyStartedChildRun(ctx, started); err != nil {
			return err
		}
	}
	for _, aborted := range tree.AbortedChildren {
		record, err := childRunStartReservationRecord(aborted)
		if err != nil {
			return err
		}
		if _, err := e.childRunStarts.Conclude(ctx, record, runs.ChildRunStartAborted); err != nil {
			return err
		}
	}
	for _, commit := range tree.Events {
		if err := e.applyCommit(ctx, commit); err != nil {
			return err
		}
	}
	return nil
}

func (e *Effects) reconcileExecutionTree(readCtx context.Context, tree runs.ExecutionTreeCommit, err error) error {
	update, commits := tree.Update, tree.Events
	head, found, readErr := e.executionTrees.LoadExecutionTree(readCtx, update.Head.SessionID, update.Head.RootID)
	if readErr != nil || !found || !head.SameCommit(update.Head) {
		return errors.Join(err, readErr)
	}
	for _, acceptance := range tree.InputAcceptances {
		record, lookupErr := e.commandAcceptances.Lookup(readCtx, acceptance.Key)
		if lookupErr != nil || !record.Found || !record.Acceptance.Equal(acceptance) {
			return errors.Join(err, lookupErr)
		}
	}
	for _, started := range tree.StartedChildren {
		confirmed, lookupErr := e.reconcileOpeningCommit(readCtx, started.Opening())
		if lookupErr != nil || !confirmed {
			return errors.Join(err, lookupErr)
		}
	}
	for _, aborted := range tree.AbortedChildren {
		record, recordErr := childRunStartReservationRecord(aborted)
		if recordErr != nil {
			return errors.Join(err, recordErr)
		}
		confirmed, lookupErr := e.childRunStarts.Concluded(readCtx, record, runs.ChildRunStartAborted)
		if lookupErr != nil || !confirmed {
			return errors.Join(err, lookupErr)
		}
	}
	return e.reconcileExecutionTreeEvents(readCtx, commits, err)
}

func (e *Effects) reconcileExecutionTreeEvents(readCtx context.Context, commits []runs.EventCommit, err error) error {
	last := make(map[string]runs.EventCommit)
	for _, commit := range commits {
		last[commit.RunID] = commit
	}
	for _, commit := range last {
		if commit.ResultPublication != nil {
			continue
		}
		confirmed, lookupErr := e.runState.RunCommitCommitted(readCtx, commit.SessionID, commit.RunID, commit.SegmentID, commit.CommitID)
		if lookupErr != nil || !confirmed {
			return errors.Join(err, lookupErr)
		}
	}
	for _, commit := range commits {
		if commit.ResultPublication == nil {
			continue
		}
		var confirmed bool
		var lookupErr error
		if last[commit.RunID].State == runs.StateTerminalize {
			// The exact terminal receipt above fences this now-closed Segment.
			confirmed, lookupErr = e.executionTrees.ExecutionResultCommitted(readCtx, commit.SessionID, *commit.ResultPublication)
		} else {
			confirmed, lookupErr = e.runState.ResultPublicationCommitted(readCtx, commit.SessionID, commit.RunID, commit.SegmentID, commit.ResultPublication.ID, commit.ResultPublication.Digest)
		}
		if lookupErr != nil || !confirmed {
			return errors.Join(err, lookupErr)
		}
	}
	return nil
}
