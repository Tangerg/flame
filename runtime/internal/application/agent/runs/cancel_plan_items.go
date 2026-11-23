package runs

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
)

func (c *Cancellations) loadWaitingCancellationItems(ctx context.Context, plan *cancellationPlan) error {
	if plan == nil {
		return errors.New("runs: waiting cancellation plan is required")
	}
	if err := c.loadWaitingCancellationSpawningItem(ctx, plan); err != nil {
		return err
	}
	itemsByID, err := pendingItems(ctx, c.items, *plan.pending)
	if err != nil {
		return err
	}
	interrupts, err := plan.pending.ProjectInterrupts(itemsByID)
	if err != nil {
		return fmt.Errorf("runs: waiting cancellation: %w", err)
	}
	plan.items, plan.interrupts = itemsByID, interrupts
	targetRunIDs := cancellationTargetRunIDs(plan.targetSubtree)
	for _, request := range interrupts {
		if _, targeted := targetRunIDs[request.RunID]; targeted {
			plan.targetInterruptItems = append(plan.targetInterruptItems, itemsByID[request.ItemID])
		}
	}
	for _, continuation := range plan.pending.Continuations {
		if _, targeted := targetRunIDs[continuation.RunID]; !targeted {
			continue
		}
		for _, drained := range continuation.DrainedTools {
			item, err := drainedToolItem(itemsByID, continuation.RunID, drained)
			if err != nil {
				return fmt.Errorf("runs: waiting cancellation: %w", err)
			}
			plan.targetDrainedItems = append(plan.targetDrainedItems, item)
		}
	}
	return nil
}

func pendingItems(ctx context.Context, items ItemProjection, pending Pending) (map[identity.ItemID]transcript.Item, error) {
	itemsByID := make(map[identity.ItemID]transcript.Item)
	for _, itemID := range pendingItemIDs(pending) {
		item, found, err := items.Item(ctx, itemID)
		if err != nil {
			return nil, fmt.Errorf("runs: read waiting Item %q: %w", itemID, err)
		}
		if !found {
			return nil, fmt.Errorf("runs: waiting Item %q is missing", itemID)
		}
		itemsByID[itemID] = item
	}
	return itemsByID, nil
}

func (c *Cancellations) loadWaitingCancellationSpawningItem(
	ctx context.Context,
	plan *cancellationPlan,
) error {
	item, found, err := c.items.Item(ctx, plan.target.run.Lineage().SpawnedByItemID())
	if err != nil {
		return fmt.Errorf(
			"runs: read spawning Item %q: %w",
			plan.target.run.Lineage().SpawnedByItemID(),
			err,
		)
	}
	if !found {
		return fmt.Errorf(
			"runs: waiting child Run %q spawning Item %q is missing",
			plan.target.run.ID(),
			plan.target.run.Lineage().SpawnedByItemID(),
		)
	}
	if err := validateWaitingCancellationSpawningItem(*plan, item); err != nil {
		return err
	}
	plan.spawningItem = &item
	return nil
}

func cancellationTargetRunIDs(subtree []cancellationRun) map[identity.RunID]struct{} {
	targetRunIDs := make(map[identity.RunID]struct{}, len(subtree))
	for _, member := range subtree {
		targetRunIDs[member.run.ID()] = struct{}{}
	}
	return targetRunIDs
}

func validateWaitingCancellationSpawningItem(plan cancellationPlan, item transcript.Item) error {
	if err := item.ValidateSpawns(plan.root.run.SessionID(), plan.target.run.Lineage().ParentRunID()); err != nil {
		return fmt.Errorf("runs: waiting child Run %q: %w", plan.target.run.ID(), err)
	}
	switch {
	case item.ID() != plan.target.run.Lineage().SpawnedByItemID():
		return fmt.Errorf(
			"runs: waiting child Run %q resolved spawning Item %q, want %q",
			plan.target.run.ID(),
			item.ID(),
			plan.target.run.Lineage().SpawnedByItemID(),
		)
	case item.Status() != transcript.ItemRunning:
		return fmt.Errorf(
			"runs: spawning Item %q is in status %s, want running",
			item.ID(),
			item.Status(),
		)
	}
	call, present := item.ToolCall()
	if !present {
		return fmt.Errorf("runs: spawning Item %q has no tool invocation", item.ID())
	}
	if call.Failure != nil {
		return fmt.Errorf("runs: spawning Item %q already carries a failure", item.ID())
	}
	return nil
}
