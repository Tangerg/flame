package persistence

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/application/agent/sessions"
	"github.com/Tangerg/flame/runtime/internal/infra/sqlite"
)

// WorkspaceMutationStore translates the Session rollback use case's durable
// intent into a SQLite technical record. The transaction ordering remains an
// Application concern; SQLite only persists and lists records.
type WorkspaceMutationStore struct {
	storage *sqlite.WorkspaceMutationStore
}

func NewWorkspaceMutationStore(storage *sqlite.WorkspaceMutationStore) *WorkspaceMutationStore {
	return &WorkspaceMutationStore{storage: storage}
}

func (w *WorkspaceMutationStore) Record(ctx context.Context, mutation sessions.WorkspaceMutation) (bool, error) {
	created, err := w.storage.Record(ctx, storedWorkspaceMutation(mutation))
	if errors.Is(err, sqlite.ErrWorkspaceMutationPending) {
		return false, fmt.Errorf("%w: %w", sessions.ErrWorkspaceMutationPending, err)
	}
	return created, err
}

func (w *WorkspaceMutationStore) Complete(ctx context.Context, mutation sessions.WorkspaceMutation) error {
	return w.storage.Complete(ctx, storedWorkspaceMutation(mutation))
}

func (w *WorkspaceMutationStore) ListPending(ctx context.Context) ([]sessions.WorkspaceMutation, error) {
	records, err := w.storage.ListPending(ctx)
	if err != nil {
		return nil, err
	}
	mutations := make([]sessions.WorkspaceMutation, len(records))
	for index, record := range records {
		mutations[index] = sessions.WorkspaceMutation{
			SessionID:      record.SessionID,
			CWD:            record.CWD,
			ToRunID:        record.ToRunID,
			RestoreHistory: record.RestoreHistory,
		}
	}
	return mutations, nil
}

func storedWorkspaceMutation(mutation sessions.WorkspaceMutation) sqlite.WorkspaceMutationRecord {
	return sqlite.WorkspaceMutationRecord{
		SessionID:      mutation.SessionID,
		CWD:            mutation.CWD,
		ToRunID:        mutation.ToRunID,
		RestoreHistory: mutation.RestoreHistory,
	}
}
