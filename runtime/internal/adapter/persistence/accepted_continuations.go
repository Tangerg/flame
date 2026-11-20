package persistence

import (
	"context"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

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
		Segments:    encodeAcceptedSegments(accepted.Segments),
	}
	return s.saveContinuation(ctx, record)
}

func (s *acceptedContinuationStore) List(ctx context.Context) ([]runs.AcceptedContinuation, error) {
	records, err := s.continuationRecords(ctx, `WHERE canceled_run_id IS NULL`)
	if err != nil {
		return nil, err
	}
	values := make([]runs.AcceptedContinuation, 0, len(records))
	for _, record := range records {
		pending, err := pendingValue(record.Interrupt)
		if err != nil {
			return nil, err
		}
		value := runs.AcceptedContinuation{
			SessionID: record.SessionID, Pending: pending, InputItemID: record.InputItemID,
			Segments: decodeAcceptedSegments(record.Segments),
		}
		if err := value.Validate(); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, nil
}

func (s *acceptedContinuationStore) PutSubtreeCancellation(ctx context.Context, accepted runs.AcceptedSubtreeCancellation) error {
	if err := accepted.Validate(); err != nil {
		return err
	}
	return s.saveContinuation(ctx, acceptedContinuationRecord{
		SessionID: accepted.SessionID, Interrupt: interruptRecord(accepted.Pending),
		Segments: encodeAcceptedSegments(accepted.Segments), CanceledRunID: accepted.TargetRunID,
	})
}

func (s *acceptedContinuationStore) ListSubtreeCancellations(ctx context.Context) ([]runs.AcceptedSubtreeCancellation, error) {
	return s.subtreeCancellations(ctx, `WHERE canceled_run_id IS NOT NULL`)
}

func (s *acceptedContinuationStore) SubtreeCancellations(ctx context.Context, rootRunID identity.RunID) ([]runs.AcceptedSubtreeCancellation, error) {
	return s.subtreeCancellations(ctx, `WHERE canceled_run_id IS NOT NULL AND root_run_id = ?`, rootRunID)
}

func (s *acceptedContinuationStore) subtreeCancellations(ctx context.Context, filter string, args ...any) ([]runs.AcceptedSubtreeCancellation, error) {
	records, err := s.continuationRecords(ctx, filter, args...)
	if err != nil {
		return nil, err
	}
	values := make([]runs.AcceptedSubtreeCancellation, 0, len(records))
	for _, record := range records {
		pending, err := pendingValue(record.Interrupt)
		if err != nil {
			return nil, err
		}
		value := runs.AcceptedSubtreeCancellation{
			SessionID: record.SessionID, Pending: pending, TargetRunID: record.CanceledRunID,
			Segments: decodeAcceptedSegments(record.Segments),
		}
		if err := value.Validate(); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, nil
}

func encodeAcceptedSegments(segments []runs.AcceptedSegment) []acceptedSegmentRecord {
	records := make([]acceptedSegmentRecord, len(segments))
	for index, segment := range segments {
		records[index] = acceptedSegmentRecord{RunID: segment.RunID, SegmentID: segment.SegmentID}
	}
	return records
}

func decodeAcceptedSegments(records []acceptedSegmentRecord) []runs.AcceptedSegment {
	segments := make([]runs.AcceptedSegment, len(records))
	for index, record := range records {
		segments[index] = runs.AcceptedSegment{RunID: record.RunID, SegmentID: record.SegmentID}
	}
	return segments
}
