package persistence

import (
	"context"
	"errors"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
)

func (s *ExecutorCheckpointStore) LoadExecutionTree(ctx context.Context, sessionID, rootID string) (runs.ExecutionTreeHead, bool, error) {
	value, found, err := s.loadExecutionTreeRecord(ctx, sessionID, rootID)
	return runs.ExecutionTreeHead{SessionID: value.SessionID, RootID: value.RootID, Writer: value.Writer, Digest: value.Digest, Payload: value.Payload, Sequence: value.Sequence, CommitID: value.CommitID, CommitDigest: value.CommitDigest}, found, err
}

func (s *ExecutorCheckpointStore) ExecutionResultCommitted(ctx context.Context, sessionID string, publication runs.ResultPublication) (bool, error) {
	return s.executionResultCommitted(ctx, sessionID, publication.ID, publication.Digest)
}

func (s *ExecutorCheckpointStore) SaveExecutionTree(ctx context.Context, update runs.ExecutionTreeUpdate) error {
	if err := update.Validate(); err != nil {
		return err
	}
	h := update.Head
	err := s.saveExecutionTreeRecord(ctx, update.PreviousWriter, update.PreviousDigest, executionTreeRecord{SessionID: h.SessionID, RootID: h.RootID, Writer: h.Writer, Digest: h.Digest, Payload: h.Payload, Sequence: h.Sequence, CommitID: h.CommitID, CommitDigest: h.CommitDigest})
	switch {
	case errors.Is(err, errExecutionTreeCommitConflict):
		return errors.Join(runs.ErrExecutionTreeCommitConflict, err)
	case errors.Is(err, errExecutionTreeConflict):
		return errors.Join(runs.ErrExecutionTreeWriterConflict, err)
	default:
		return err
	}
}
