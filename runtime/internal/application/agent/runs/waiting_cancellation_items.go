package runs

import (
	"context"
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	rundomain "github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
)

func (c *Cancellations) loadWaitingCancellationItems(ctx context.Context, plan *waitingCancellationPlan) error {
	itemsByID, err := pendingItems(ctx, c.items, plan.pending)
	if err != nil {
		return err
	}
	spawningItemID := plan.target.Lineage().SpawnedByItemID()
	spawningItem, found := itemsByID[spawningItemID]
	if !found {
		return fmt.Errorf("runs: waiting child Run %q spawning Item %q is missing from Pending", plan.target.ID(), spawningItemID)
	}
	if err := validateWaitingCancellationSpawningItem(*plan, spawningItem); err != nil {
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

func cancellationTargetRunIDs(subtree []rundomain.Run) map[identity.RunID]struct{} {
	targetRunIDs := make(map[identity.RunID]struct{}, len(subtree))
	for _, member := range subtree {
		targetRunIDs[member.ID()] = struct{}{}
	}
	return targetRunIDs
}

func validateWaitingCancellationSpawningItem(plan waitingCancellationPlan, item transcript.Item) error {
	if err := item.ValidateSpawns(plan.root.SessionID(), plan.target.Lineage().ParentRunID()); err != nil {
		return fmt.Errorf("runs: waiting child Run %q: %w", plan.target.ID(), err)
	}
	switch {
	case item.ID() != plan.target.Lineage().SpawnedByItemID():
		return fmt.Errorf(
			"runs: waiting child Run %q resolved spawning Item %q, want %q",
			plan.target.ID(),
			item.ID(),
			plan.target.Lineage().SpawnedByItemID(),
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
