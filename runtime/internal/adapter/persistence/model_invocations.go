package persistence

import (
	"context"
	"errors"
	"fmt"

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
		records[index], err = modelInvocationFromRecord(row)
		if err != nil {
			return nil, err
		}
	}
	return records, nil
}

func modelInvocationFromRecord(row sqlite.ModelInvocationRecord) (runs.ModelInvocationCommit, error) {
	record := runs.ModelInvocationCommit{
		FirstOutputLatencyMillis: row.FirstOutputLatencyMillis, Usage: row.Usage,
		CallID: row.CallID, SegmentID: row.SegmentID, State: runs.ModelInvocationState(row.State),
		StartedAt: row.StartedAt, FinishedAt: row.FinishedAt,
	}
	if err := record.Validate(); err != nil {
		return runs.ModelInvocationCommit{}, fmt.Errorf("persistence: restore model invocation: %w", err)
	}
	return record, nil
}
