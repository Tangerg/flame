package persistence

import (
	"context"
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/fingerprint"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
)

func (s *InterruptStore) Open(ctx context.Context, pending runs.Pending) error {
	if err := pending.Validate(); err != nil {
		return err
	}
	return s.insertPending(ctx, interruptRecord(pending))
}

func (s *InterruptStore) List(ctx context.Context, sessionID string) ([]runs.Pending, error) {
	records, err := s.pendingRecords(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	return pendingValues(records)
}

func (s *InterruptStore) ListPage(
	ctx context.Context,
	sessionID, rootRunID string,
	afterCreatedAt int64,
	afterRootRunID string,
	limit int,
) ([]runs.Pending, error) {
	records, err := s.pendingRecordsPage(
		ctx,
		sessionID,
		rootRunID,
		afterCreatedAt,
		afterRootRunID,
		limit,
	)
	if err != nil {
		return nil, err
	}
	return pendingValues(records)
}

func (s *InterruptStore) Get(ctx context.Context, rootRunID string) (runs.Pending, bool, error) {
	record, ok, err := s.loadPending(ctx, rootRunID)
	if err != nil || !ok {
		return runs.Pending{}, ok, err
	}
	pending, err := pendingValue(record)
	if err != nil {
		return runs.Pending{}, false, err
	}
	if err := pending.Validate(); err != nil {
		return runs.Pending{}, false, err
	}
	return pending, true, nil
}

func (s *InterruptStore) Consume(ctx context.Context, rootRunID string) (runs.Pending, bool, error) {
	record, ok, err := s.consumePending(ctx, rootRunID)
	if err != nil || !ok {
		return runs.Pending{}, ok, err
	}
	pending, err := pendingValue(record)
	if err != nil {
		return runs.Pending{}, false, err
	}
	if err := pending.Validate(); err != nil {
		return runs.Pending{}, false, err
	}
	return pending, true, nil
}

func pendingValues(records []pendingRecord) ([]runs.Pending, error) {
	values := make([]runs.Pending, len(records))
	for index, record := range records {
		value, err := pendingValue(record)
		if err != nil {
			return nil, err
		}
		if err := value.Validate(); err != nil {
			return nil, err
		}
		values[index] = value
	}
	return values, nil
}

func interruptRecord(pending runs.Pending) pendingRecord {
	continuations := make([]continuationRecord, len(pending.Continuations))
	for index, continuation := range pending.Continuations {
		drained := make([]drainedToolRecord, len(continuation.DrainedTools))
		for toolIndex, tool := range continuation.DrainedTools {
			drained[toolIndex] = drainedToolRecord(tool)
		}
		continuations[index] = continuationRecord{
			RunID: continuation.RunID, MemberID: continuation.MemberID,
			DrainedTools: drained,
		}
	}
	bindings := make([]interruptBindingRecord, len(pending.Bindings))
	for index, binding := range pending.Bindings {
		bindings[index] = interruptBindingRecord{
			InterruptItemID:   binding.InterruptItemID,
			MemberID:          binding.MemberID,
			RequestID:         binding.RequestID,
			ToolCallID:        binding.ToolCallID,
			ToolRef:           binding.ToolRef.String(),
			SourceFingerprint: binding.SourceFingerprint.String(),
		}
	}
	interrupts := make([]openInterruptRecord, len(pending.Interrupts))
	for index, open := range pending.Interrupts {
		interrupts[index] = openInterruptRecord{ItemID: open.ItemID}
		if review := open.Approval; review != nil {
			interrupts[index].Approval = &approvalReviewRecord{
				Risk: review.Risk, Reason: review.Reason, Rememberable: review.Rememberable,
			}
		}
	}
	return pendingRecord{
		RootRunID: pending.RootRunID, ExecutorID: pending.ExecutorID,
		Interrupts: interrupts, Bindings: bindings,
		Continuations: continuations,
		CreatedAt:     pending.CreatedAt,
	}
}

func pendingValue(record pendingRecord) (runs.Pending, error) {
	continuations := make([]runs.Continuation, len(record.Continuations))
	for index, continuation := range record.Continuations {
		var drained []runs.DrainedTool
		if len(continuation.DrainedTools) > 0 {
			drained = make([]runs.DrainedTool, len(continuation.DrainedTools))
		}
		for toolIndex, tool := range continuation.DrainedTools {
			drained[toolIndex] = runs.DrainedTool(tool)
		}
		continuations[index] = runs.Continuation{
			RunID: continuation.RunID, MemberID: continuation.MemberID,
			DrainedTools: drained,
		}
	}
	bindings := make([]runs.InterruptBinding, len(record.Bindings))
	for index, binding := range record.Bindings {
		value := runs.InterruptBinding{
			InterruptItemID: binding.InterruptItemID,
			MemberID:        binding.MemberID,
			RequestID:       binding.RequestID,
			ToolCallID:      binding.ToolCallID,
		}
		if binding.ToolRef != "" {
			ref, err := tool.ParseRef(binding.ToolRef)
			if err != nil {
				return runs.Pending{}, fmt.Errorf("persistence: decode interrupt binding tool ref: %w", err)
			}
			value.ToolRef = ref
		}
		if binding.SourceFingerprint != "" {
			digest, err := fingerprint.ParseDigest(binding.SourceFingerprint)
			if err != nil {
				return runs.Pending{}, fmt.Errorf("persistence: decode interrupt binding source fingerprint: %w", err)
			}
			value.SourceFingerprint = digest
		}
		bindings[index] = value
	}
	interrupts := make([]runs.OpenInterrupt, len(record.Interrupts))
	for index, open := range record.Interrupts {
		interrupts[index] = runs.OpenInterrupt{ItemID: open.ItemID}
		if review := open.Approval; review != nil {
			interrupts[index].Approval = &runs.ApprovalReview{
				Risk: review.Risk, Reason: review.Reason, Rememberable: review.Rememberable,
			}
		}
	}
	return runs.Pending{
		RootRunID: record.RootRunID, ExecutorID: record.ExecutorID,
		Interrupts: interrupts, Bindings: bindings,
		Continuations: continuations,
		CreatedAt:     record.CreatedAt,
	}, nil
}
