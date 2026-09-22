package persistence

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/infra/sqlite"
)

// ExecutorCheckpointStore translates the Application-owned checkpoint
// aggregate to SQLite's technical record. SQLite never imports Application and
// never becomes an owner of executor lifecycle semantics.
type ExecutorCheckpointStore struct {
	storage *sqlite.ExecutorCheckpointStore
}

func NewExecutorCheckpointStore(storage *sqlite.ExecutorCheckpointStore) *ExecutorCheckpointStore {
	return &ExecutorCheckpointStore{storage: storage}
}

func (e *ExecutorCheckpointStore) LoadExecutionTree(ctx context.Context, sessionID, rootID string) (runs.ExecutionTreeHead, bool, error) {
	value, found, err := e.storage.LoadExecutionTree(ctx, sessionID, rootID)
	return runs.ExecutionTreeHead{SessionID: value.SessionID, RootID: value.RootID, Writer: value.Writer, Digest: value.Digest, Payload: value.Payload, Sequence: value.Sequence, CommitID: value.CommitID, CommitDigest: value.CommitDigest}, found, err
}

func (e *ExecutorCheckpointStore) ExecutionResultCommitted(ctx context.Context, sessionID string, publication runs.ResultPublication) (bool, error) {
	return e.storage.ExecutionResultCommitted(ctx, sessionID, publication.ID, publication.Digest)
}

func (e *ExecutorCheckpointStore) SaveExecutionTree(ctx context.Context, update runs.ExecutionTreeUpdate) error {
	if err := update.Validate(); err != nil {
		return err
	}
	h := update.Head
	err := e.storage.SaveExecutionTree(ctx, update.PreviousWriter, update.PreviousDigest, sqlite.ExecutionTreeRecord{SessionID: h.SessionID, RootID: h.RootID, Writer: h.Writer, Digest: h.Digest, Payload: h.Payload, Sequence: h.Sequence, CommitID: h.CommitID, CommitDigest: h.CommitDigest})
	switch {
	case errors.Is(err, sqlite.ErrExecutionTreeCommitConflict):
		return errors.Join(runs.ErrExecutionTreeCommitConflict, err)
	case errors.Is(err, sqlite.ErrExecutionTreeConflict):
		return errors.Join(runs.ErrExecutionTreeWriterConflict, err)
	default:
		return err
	}
}

func (e *ExecutorCheckpointStore) SaveCheckpoint(ctx context.Context, checkpoint runs.ExecutorCheckpoint) error {
	if err := checkpoint.Validate(); err != nil {
		return err
	}
	err := e.storage.SaveCheckpoint(ctx, sqlite.ExecutorCheckpointRecord{
		ToolResultIDs: slices.Clone(checkpoint.ToolResultIDs),
		RootMemberID:  checkpoint.RootMemberID,
		Payload:       append([]byte(nil), checkpoint.Payload...),
		BuildID:       checkpoint.BuildID,
		Scope: sqlite.ExecutorScopeRecord{
			SessionID:         checkpoint.Scope.SessionID,
			CWD:               checkpoint.Scope.CWD,
			WorkspaceCWD:      checkpoint.Scope.WorkspaceCWD,
			Isolated:          checkpoint.Scope.Isolated,
			GoalIncarnationID: checkpoint.Scope.GoalIncarnationID,
		},
		ModelSelection: checkpoint.ModelSelection,
		Capabilities:   checkpoint.Capabilities.Clone(),
		Usage:          checkpoint.Usage,
	})
	return translateCheckpointStorageError(err)
}

func (e *ExecutorCheckpointStore) LoadCheckpoint(ctx context.Context, rootMemberID string) (runs.ExecutorCheckpoint, error) {
	record, err := e.storage.LoadCheckpoint(ctx, rootMemberID)
	if err != nil {
		return runs.ExecutorCheckpoint{}, translateCheckpointStorageError(err)
	}
	checkpoint := runs.ExecutorCheckpoint{
		ToolResultIDs: slices.Clone(record.ToolResultIDs),
		RootMemberID:  record.RootMemberID,
		Payload:       append([]byte(nil), record.Payload...),
		BuildID:       record.BuildID,
		Scope: runs.ExecutionScope{
			SessionID:         record.Scope.SessionID,
			CWD:               record.Scope.CWD,
			WorkspaceCWD:      record.Scope.WorkspaceCWD,
			Isolated:          record.Scope.Isolated,
			GoalIncarnationID: record.Scope.GoalIncarnationID,
		},
		ModelSelection: record.ModelSelection,
		Capabilities:   record.Capabilities.Clone(),
		Usage:          record.Usage,
	}
	if err := checkpoint.Validate(); err != nil {
		return runs.ExecutorCheckpoint{}, fmt.Errorf("persistence: load executor checkpoint: %w", err)
	}
	return checkpoint, nil
}

func (e *ExecutorCheckpointStore) DeleteCheckpoints(ctx context.Context, sessionID string, rootIDs []string) error {
	return translateCheckpointStorageError(e.storage.DeleteCheckpoints(ctx, sessionID, rootIDs))
}

func (e *ExecutorCheckpointStore) DeleteSessionCheckpoints(ctx context.Context, sessionID string) error {
	return translateCheckpointStorageError(e.storage.DeleteSessionCheckpoints(ctx, sessionID))
}

func translateCheckpointStorageError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, sqlite.ErrExecutorCheckpointRecordNotFound):
		return fmt.Errorf("%w: %w", runs.ErrExecutorCheckpointNotFound, err)
	case errors.Is(err, sqlite.ErrInvalidExecutorCheckpointRecord):
		return fmt.Errorf("%w: %w", runs.ErrInvalidExecutorCheckpoint, err)
	default:
		return err
	}
}
