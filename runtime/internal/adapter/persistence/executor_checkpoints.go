package persistence

import (
	"context"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/infra/sqlite"
)

// ExecutorCheckpointStore translates Application execution-tree commits to SQLite.
type ExecutorCheckpointStore struct {
	*sqlite.ExecutorCheckpointStore
}

func NewExecutorCheckpointStore(storage *sqlite.ExecutorCheckpointStore) *ExecutorCheckpointStore {
	return &ExecutorCheckpointStore{ExecutorCheckpointStore: storage}
}

func (e *ExecutorCheckpointStore) LoadExecutionTree(ctx context.Context, sessionID, rootID string) (runs.ExecutionTreeHead, bool, error) {
	value, found, err := e.ExecutorCheckpointStore.LoadExecutionTree(ctx, sessionID, rootID)
	return runs.ExecutionTreeHead{SessionID: value.SessionID, RootID: value.RootID, Writer: value.Writer, Digest: value.Digest, Payload: value.Payload}, found, err
}

func (e *ExecutorCheckpointStore) ExecutionResultCommitted(ctx context.Context, sessionID string, publication runs.ResultPublication) (bool, error) {
	return e.ExecutorCheckpointStore.ExecutionResultCommitted(ctx, sessionID, publication.ID, publication.Digest)
}

func (e *ExecutorCheckpointStore) SaveExecutionTree(ctx context.Context, update runs.ExecutionTreeUpdate) error {
	if err := update.Validate(); err != nil {
		return err
	}
	h := update.Head
	return e.ExecutorCheckpointStore.SaveExecutionTree(ctx, update.PreviousWriter, update.PreviousDigest, sqlite.ExecutionTreeRecord{SessionID: h.SessionID, RootID: h.RootID, Writer: h.Writer, Digest: h.Digest, Payload: h.Payload})
}
