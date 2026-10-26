package persistence

import (
	"context"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
)

func (s *acceptedContinuationStore) Put(ctx context.Context, accepted runs.AcceptedContinuation) error {
	if err := accepted.Validate(); err != nil {
		return err
	}
	record := acceptedContinuationRecord{
		SessionID:   accepted.SessionID,
		Interrupt:   interruptRecord(accepted.Pending),
		InputItemID: accepted.InputItemID,
	}
	for _, segment := range accepted.Segments {
		record.Segments = append(record.Segments, acceptedSegmentRecord{RunID: segment.RunID, SegmentID: segment.SegmentID})
	}
	return s.saveContinuation(ctx, record)
}

func (s *acceptedContinuationStore) List(ctx context.Context) ([]runs.AcceptedContinuation, error) {
	records, err := s.continuationRecords(ctx)
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
