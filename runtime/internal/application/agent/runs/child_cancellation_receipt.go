package runs

import (
	"context"
	"errors"
	"slices"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
)

type childCancellationCommit struct {
	executor ExecutorRef
	result   CancelResult
	receipt  CancellationReceipt
}

func (c childCancellationCommit) validate(commit EventCommit) error {
	child, root := c.result.Run, c.result.RootRun
	lineage := child.Lineage()
	if c.receipt == nil || c.executor.ExecutorID == "" || c.executor.SessionID != commit.SessionID || child.SessionID() != commit.SessionID ||
		root == nil || !root.Lineage().IsRoot() || root.ID() != lineage.RootRunID() || root.SessionID() != commit.SessionID ||
		!child.Canceled() || lineage.ParentRunID() != commit.RunID ||
		!settlesChildCancellationItem(commit.Items, lineage.SpawnedByItemID()) {
		return errors.New("runs: child cancellation acknowledgement requires its terminal target and spawning Item")
	}
	return nil
}

func settlesChildCancellationItem(items []ItemWrite, itemID identity.ItemID) bool {
	return slices.ContainsFunc(items, func(write ItemWrite) bool {
		item := write.Item()
		return item.ID() == itemID && item.Kind() == transcript.ToolCall && item.Status() == transcript.ItemIncomplete
	})
}

func (r *runTreeOwner) bindChildCancellationReceipt(commit *EventCommit, prior []EventCommit) {
	r.mu.Lock()
	defer r.mu.Unlock()
	attempt := r.childCancel
	if attempt == nil || attempt.receipt == nil || attempt.parentRunID != commit.RunID ||
		!settlesChildCancellationItem(commit.Items, attempt.spawningItemID) {
		return
	}
	if terminal, confirmed := r.childCancellationTerminalLocked(attempt, prior); confirmed {
		commit.childCancellation = &childCancellationCommit{
			executor: attempt.executor, result: attempt.result(terminal), receipt: attempt.receipt,
		}
	}
}

func (e EventCommit) RecordChildCancellation(ctx context.Context) error {
	if e.childCancellation == nil {
		return nil
	}
	return e.childCancellation.receipt.Stage(ctx, e.childCancellation.executor, func(context.Context) (CancelResult, error) {
		return e.childCancellation.result, nil
	})
}
