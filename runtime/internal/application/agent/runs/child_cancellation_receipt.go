package runs

import (
	"context"
	"errors"
	"slices"

	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
)

type childCancellationCommit struct {
	executor ExecutorRef
	result   CancelResult
	record   ChildCancellationReceipt
}

func (c childCancellationCommit) validate(commit EventCommit) error {
	child, root := c.result.Run, c.result.RootRun
	lineage := child.Lineage()
	if c.record == nil || c.executor.ExecutorID == "" || c.executor.SessionID != commit.SessionID || child.SessionID() != commit.SessionID ||
		root == nil || !root.Lineage().IsRoot() || root.ID() != lineage.RootRunID() || root.SessionID() != commit.SessionID ||
		!child.Canceled() || lineage.ParentRunID() != commit.RunID ||
		!settlesChildCancellationItem(commit.Items, lineage.SpawnedByItemID()) {
		return errors.New("runs: child cancellation acknowledgement requires its terminal target and spawning Item")
	}
	return nil
}

func settlesChildCancellationItem(items []ItemWrite, itemID string) bool {
	return slices.ContainsFunc(items, func(write ItemWrite) bool {
		item := write.Item()
		return item.ID() == itemID && item.Kind() == transcript.ToolCall && item.Status() == transcript.ItemIncomplete
	})
}

func (r *runTreeOwner) bindChildCancellationReceipt(commit *EventCommit, prior []EventCommit) {
	r.mu.Lock()
	defer r.mu.Unlock()
	attempt := r.childCancel
	if attempt == nil || attempt.record == nil || attempt.parentRunID != commit.RunID ||
		!settlesChildCancellationItem(commit.Items, attempt.spawningItemID) {
		return
	}
	if terminal := attempt.terminalBefore(prior); terminal != nil {
		commit.childCancellation = &childCancellationCommit{
			executor: attempt.executor, result: attempt.result(*terminal), record: attempt.record,
		}
	}
}

func (e EventCommit) RecordChildCancellation(ctx context.Context) error {
	if e.childCancellation == nil {
		return nil
	}
	receipt := e.childCancellation
	return receipt.record(ctx, receipt.executor, receipt.result)
}
