package segment

import (
	"context"
	"errors"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
)

func (e *Effects) CommitExecutionTree(ctx context.Context, update runs.ExecutionTreeUpdate, commits []runs.EventCommit) (commitErr error) {
	if err := update.Validate(); err != nil {
		return err
	}
	for _, commit := range commits {
		if commit.SessionID != update.Head.SessionID || commit.CommitID.IsZero() || commit.State == runs.StateSuspend || commit.ObsoleteCheckpointRootID != "" {
			return errors.New("segment: invalid execution tree product commit")
		}
		if err := commit.Validate(); err != nil {
			return err
		}
	}
	defer func() {
		if commitErr != nil {
			for _, commit := range commits {
				commitErr = e.compensateFailedCommit(ctx, commit, commitErr)
			}
		}
	}()
	err := e.runInTx(ctx, func(ctx context.Context) error {
		if err := e.executionTrees.SaveExecutionTree(ctx, update); err != nil {
			return err
		}
		for _, commit := range commits {
			if err := e.applyCommit(ctx, commit); err != nil {
				return err
			}
		}
		return nil
	})
	if err == nil {
		return nil
	}
	readCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), stagedToolResultCleanupTimeout)
	defer cancel()
	head, found, readErr := e.executionTrees.LoadExecutionTree(readCtx, update.Head.SessionID, update.Head.RootID)
	if readErr == nil && found && head.SameCommit(update.Head) {
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
	return errors.Join(err, readErr)
}
