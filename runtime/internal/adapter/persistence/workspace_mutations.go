package persistence

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/application/agent/sessions"
)

func (s *WorkspaceMutationStore) Record(ctx context.Context, mutation sessions.WorkspaceMutation) (bool, error) {
	created, err := s.recordMutation(ctx, storedWorkspaceMutation(mutation))
	if errors.Is(err, errWorkspaceMutationPending) {
		return false, fmt.Errorf("%w: %w", sessions.ErrWorkspaceMutationPending, err)
	}
	return created, err
}

func (s *WorkspaceMutationStore) Complete(ctx context.Context, mutation sessions.WorkspaceMutation) error {
	return s.completeMutation(ctx, storedWorkspaceMutation(mutation))
}

func (s *WorkspaceMutationStore) ListPending(ctx context.Context) ([]sessions.WorkspaceMutation, error) {
	records, err := s.pendingMutations(ctx)
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

func storedWorkspaceMutation(mutation sessions.WorkspaceMutation) workspaceMutationRecord {
	return workspaceMutationRecord{
		SessionID:      mutation.SessionID,
		CWD:            mutation.CWD,
		ToRunID:        mutation.ToRunID,
		RestoreHistory: mutation.RestoreHistory,
	}
}
