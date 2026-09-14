package persistence

import (
	"context"
	"errors"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/infra/sqlite"
)

// ModelInvocationReader translates the stored journal into application observations.
type ModelInvocationReader struct{ store *sqlite.ModelInvocationStore }

func NewModelInvocationReader(store *sqlite.ModelInvocationStore) (*ModelInvocationReader, error) {
	if store == nil {
		return nil, errors.New("persistence: model invocation store is required")
	}
	return &ModelInvocationReader{store: store}, nil
}

func (m ModelInvocationReader) PageModelInvocations(ctx context.Context, runID string, beforeStartedAt int64, beforeCallID string, limit int) ([]runs.ModelInvocationCommit, error) {
	rows, err := m.store.PageModelInvocations(ctx, runID, beforeStartedAt, beforeCallID, limit)
	if err != nil {
		return nil, err
	}
	records := make([]runs.ModelInvocationCommit, len(rows))
	for index, row := range rows {
		records[index] = runs.ModelInvocationCommit{Usage: row.Usage, CallID: row.CallID, SegmentID: row.SegmentID, State: runs.ModelInvocationState(row.State), StartedAt: row.StartedAt, FinishedAt: row.FinishedAt}
	}
	return records, nil
}
