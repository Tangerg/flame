package persistence

import (
	"context"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/infra/sqlite"
)

// InterruptStore translates the Application-owned waiting-tree hand-off to
// SQLite's technical record. The database adapter never imports Application or
// acquires ownership of resume semantics.
type InterruptStore struct {
	storage *sqlite.InterruptStore
}

func NewInterruptStore(storage *sqlite.InterruptStore) *InterruptStore {
	return &InterruptStore{storage: storage}
}

func (i *InterruptStore) Open(ctx context.Context, pending runs.Pending) error {
	if err := pending.Validate(); err != nil {
		return err
	}
	return i.storage.Open(ctx, interruptRecord(pending))
}

func (i *InterruptStore) List(ctx context.Context, sessionID string) ([]runs.Pending, error) {
	records, err := i.storage.List(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	return pendingValues(records)
}

func (i *InterruptStore) ListPage(
	ctx context.Context,
	sessionID, rootRunID string,
	afterCreatedAt int64,
	afterRootRunID string,
	limit int,
) ([]runs.Pending, error) {
	records, err := i.storage.ListPage(
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

func (i *InterruptStore) Get(ctx context.Context, rootRunID string) (runs.Pending, bool, error) {
	record, ok, err := i.storage.Get(ctx, rootRunID)
	if err != nil || !ok {
		return runs.Pending{}, ok, err
	}
	pending := pendingValue(record)
	if err := pending.Validate(); err != nil {
		return runs.Pending{}, false, err
	}
	return pending, true, nil
}

func (i *InterruptStore) Consume(ctx context.Context, sessionID, rootRunID string) (runs.Pending, bool, error) {
	record, ok, err := i.storage.Consume(ctx, sessionID, rootRunID)
	if err != nil || !ok {
		return runs.Pending{}, ok, err
	}
	pending := pendingValue(record)
	if err := pending.Validate(); err != nil {
		return runs.Pending{}, false, err
	}
	return pending, true, nil
}

func (i *InterruptStore) ClaimResume(ctx context.Context, sessionID, rootRunID string) (runs.Pending, bool, error) {
	record, found, err := i.storage.ClaimResume(ctx, sessionID, rootRunID)
	if err != nil || !found {
		return runs.Pending{}, found, err
	}
	pending := pendingValue(record)
	if err := pending.Validate(); err != nil {
		return runs.Pending{}, false, err
	}
	return pending, true, nil
}

func (i *InterruptStore) RequireResumeClaim(ctx context.Context, sessionID, rootRunID string) error {
	return i.storage.RequireResumeClaim(ctx, sessionID, rootRunID)
}

func (i *InterruptStore) Delete(ctx context.Context, sessionID, rootRunID string) error {
	return i.storage.Delete(ctx, sessionID, rootRunID)
}

func (i *InterruptStore) DeleteResumeClaim(
	ctx context.Context,
	sessionID, rootRunID, rootMemberID string,
) error {
	return i.storage.DeleteResumeClaim(ctx, sessionID, rootRunID, rootMemberID)
}

func pendingValues(records []sqlite.InterruptRecord) ([]runs.Pending, error) {
	values := make([]runs.Pending, len(records))
	for index, record := range records {
		values[index] = pendingValue(record)
		if err := values[index].Validate(); err != nil {
			return nil, err
		}
	}
	return values, nil
}

func interruptRecord(pending runs.Pending) sqlite.InterruptRecord {
	continuations := make([]sqlite.ContinuationRecord, len(pending.Continuations))
	for index, continuation := range pending.Continuations {
		drained := make([]sqlite.DrainedToolRecord, len(continuation.DrainedTools))
		for toolIndex, tool := range continuation.DrainedTools {
			drained[toolIndex] = sqlite.DrainedToolRecord(tool)
		}
		continuations[index] = sqlite.ContinuationRecord{
			RunID: continuation.RunID, MemberID: continuation.MemberID,
			DrainedTools: drained,
		}
	}
	bindings := make([]sqlite.InterruptBindingRecord, len(pending.Bindings))
	for index, binding := range pending.Bindings {
		bindings[index] = sqlite.InterruptBindingRecord{
			InterruptItemID: binding.InterruptItemID,
			MemberID:        binding.MemberID,
			RequestID:       binding.RequestID,
			ToolCallID:      binding.ToolCallID,
		}
	}
	interrupts := make([]sqlite.OpenInterruptRecord, len(pending.Interrupts))
	for index, open := range pending.Interrupts {
		interrupts[index] = sqlite.OpenInterruptRecord{ItemID: open.ItemID}
		if review := open.Approval; review != nil {
			interrupts[index].Approval = &sqlite.ApprovalReviewRecord{
				Risk: review.Risk, Reason: review.Reason, Rememberable: review.Rememberable,
			}
		}
	}
	return sqlite.InterruptRecord{
		RootRunID: pending.RootRunID, SessionID: pending.SessionID,
		ExecutorID: pending.ExecutorID,
		Interrupts: interrupts, Bindings: bindings,
		Continuations: continuations,
		CreatedAt:     pending.CreatedAt,
	}
}

func pendingValue(record sqlite.InterruptRecord) runs.Pending {
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
		bindings[index] = runs.InterruptBinding{
			InterruptItemID: binding.InterruptItemID,
			MemberID:        binding.MemberID,
			RequestID:       binding.RequestID,
			ToolCallID:      binding.ToolCallID,
		}
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
		RootRunID: record.RootRunID, SessionID: record.SessionID,
		ExecutorID: record.ExecutorID,
		Interrupts: interrupts, Bindings: bindings,
		Continuations: continuations,
		CreatedAt:     record.CreatedAt,
	}
}
