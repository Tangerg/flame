package persistence

import (
	"context"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/infra/sqlite"
)

// AcceptedContinuationStore translates the Application-owned accepted continuation
// binding to SQLite's technical record. It reuses the interrupt hand-off codec:
// the retained binding is the exact open interrupt set the acceptance consumed, so
// revival reconstructs the same tree. The database adapter never imports resume
// semantics.
type AcceptedContinuationStore struct {
	storage *sqlite.AcceptedContinuationStore
}

func NewAcceptedContinuationStore(storage *sqlite.AcceptedContinuationStore) *AcceptedContinuationStore {
	return &AcceptedContinuationStore{storage: storage}
}

func (a *AcceptedContinuationStore) Put(ctx context.Context, accepted runs.AcceptedContinuation) error {
	if err := accepted.Validate(); err != nil {
		return err
	}
	record := sqlite.AcceptedContinuationRecord{
		SessionID:   accepted.SessionID,
		Interrupt:   interruptRecord(accepted.Pending),
		InputItemID: accepted.InputItemID,
	}
	for _, segment := range accepted.Segments {
		record.Segments = append(record.Segments, sqlite.AcceptedSegmentRecord{RunID: segment.RunID, SegmentID: segment.SegmentID})
	}
	return a.storage.Put(ctx, record)
}

func (a *AcceptedContinuationStore) Delete(ctx context.Context, rootRunID string) error {
	return a.storage.Delete(ctx, rootRunID)
}

func (a *AcceptedContinuationStore) List(ctx context.Context) ([]runs.AcceptedContinuation, error) {
	records, err := a.storage.List(ctx)
	if err != nil {
		return nil, err
	}
	values := make([]runs.AcceptedContinuation, 0, len(records))
	for _, record := range records {
		pending, err := pendingValue(record.Interrupt)
		if err != nil {
			return nil, err
		}
		value := runs.AcceptedContinuation{SessionID: record.SessionID, Pending: pending, InputItemID: record.InputItemID}
		for _, segment := range record.Segments {
			value.Segments = append(value.Segments, runs.AcceptedSegment{RunID: segment.RunID, SegmentID: segment.SegmentID})
		}
		if err := value.Validate(); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, nil
}
