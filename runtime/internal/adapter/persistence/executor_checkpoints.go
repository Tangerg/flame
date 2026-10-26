package persistence

import (
	"context"
	"errors"
	"fmt"
	"slices"

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

func (s *ExecutorCheckpointStore) SaveCheckpoint(ctx context.Context, checkpoint runs.ExecutorCheckpoint) error {
	if err := checkpoint.Validate(); err != nil {
		return err
	}
	err := s.saveCheckpointRecord(ctx, executorCheckpointRecord{
		ToolResultIDs: slices.Clone(checkpoint.ToolResultIDs),
		Installations: slices.Clone(checkpoint.Installations),
		RootMemberID:  checkpoint.RootMemberID,
		Payload:       append([]byte(nil), checkpoint.Payload...),
		BuildID:       checkpoint.BuildID,
		SessionID:     checkpoint.SessionID,
	})
	return translateCheckpointStorageError(err)
}

func (s *ExecutorCheckpointStore) LoadCheckpoint(ctx context.Context, rootMemberID string) (runs.ExecutorCheckpoint, error) {
	record, err := s.loadCheckpointRecord(ctx, rootMemberID)
	if err != nil {
		return runs.ExecutorCheckpoint{}, translateCheckpointStorageError(err)
	}
	checkpoint := runs.ExecutorCheckpoint{
		ToolResultIDs: slices.Clone(record.ToolResultIDs),
		Installations: slices.Clone(record.Installations),
		RootMemberID:  record.RootMemberID,
		Payload:       append([]byte(nil), record.Payload...),
		BuildID:       record.BuildID,
		SessionID:     record.SessionID,
	}
	if err := checkpoint.Validate(); err != nil {
		return runs.ExecutorCheckpoint{}, fmt.Errorf("persistence: load executor checkpoint: %w", err)
	}
	return checkpoint, nil
}

func (s *ExecutorCheckpointStore) LoadSessionCheckpoint(ctx context.Context, sessionID string) (runs.ExecutorCheckpoint, bool, error) {
	record, found, err := s.loadSessionCheckpointRecord(ctx, sessionID)
	if err != nil || !found {
		return runs.ExecutorCheckpoint{}, found, translateCheckpointStorageError(err)
	}
	checkpoint := runs.ExecutorCheckpoint{ToolResultIDs: slices.Clone(record.ToolResultIDs), Installations: slices.Clone(record.Installations), RootMemberID: record.RootMemberID, SessionID: record.SessionID, Payload: slices.Clone(record.Payload), BuildID: record.BuildID}
	return checkpoint, true, checkpoint.Validate()
}

func (s *ExecutorCheckpointStore) DeleteCheckpoints(ctx context.Context, sessionID string, rootIDs []string) error {
	return translateCheckpointStorageError(s.deleteCheckpoints(ctx, sessionID, rootIDs))
}

func (s *ExecutorCheckpointStore) DeleteSessionCheckpoints(ctx context.Context, sessionID string) error {
	return translateCheckpointStorageError(s.deleteSessionCheckpoints(ctx, sessionID))
}

func translateCheckpointStorageError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, errExecutorCheckpointRecordNotFound):
		return fmt.Errorf("%w: %w", runs.ErrExecutorCheckpointNotFound, err)
	case errors.Is(err, errInvalidExecutorCheckpointRecord):
		return fmt.Errorf("%w: %w", runs.ErrInvalidExecutorCheckpoint, err)
	default:
		return err
	}
}
