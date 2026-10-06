package runs

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
)

func (c *Coordinator) loadWaitingCancellationItems(ctx context.Context, plan *cancellationPlan) error {
	if plan == nil {
		return errors.New("runs: waiting cancellation plan is required")
	}
	if err := c.loadWaitingCancellationSpawningItem(ctx, plan); err != nil {
		return err
	}
	itemsByID, err := c.pendingItems(ctx, plan.pending)
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
			item, _, err := drainedToolItem(itemsByID, continuation.RunID, drained)
			if err != nil {
				return fmt.Errorf("runs: waiting cancellation: %w", err)
			}
			plan.targetDrainedItems = append(plan.targetDrainedItems, item)
		}
	}
	return nil
}

// pendingItems reads every Item a hand-off names; those Items, not the
// hand-off, own what each interrupt asked and each drained Tool invoked.
func (c *Coordinator) pendingItems(ctx context.Context, pending Pending) (map[string]transcript.Item, error) {
	itemsByID := make(map[string]transcript.Item)
	for _, itemID := range pendingItemIDs(pending) {
		item, found, err := c.items.Item(ctx, itemID)
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

func (c *Coordinator) loadWaitingCancellationSpawningItem(
	ctx context.Context,
	plan *cancellationPlan,
) error {
	item, found, err := c.items.Item(ctx, plan.target.run.Lineage().SpawnedByItemID)
	if err != nil {
		return fmt.Errorf(
			"runs: read spawning Item %q: %w",
			plan.target.run.Lineage().SpawnedByItemID,
			err,
		)
	}
	if !found {
		return fmt.Errorf(
			"runs: waiting child Run %q spawning Item %q is missing",
			plan.target.run.ID(),
			plan.target.run.Lineage().SpawnedByItemID,
		)
	}
	if err := validateWaitingCancellationSpawningItem(*plan, item); err != nil {
		return err
	}
	plan.spawningItem = item
	plan.hasSpawningItem = true
	return nil
}

func cancellationTargetRunIDs(subtree []cancellationRun) map[string]struct{} {
	targetRunIDs := make(map[string]struct{}, len(subtree))
	for _, member := range subtree {
		targetRunIDs[member.run.ID()] = struct{}{}
	}
	return targetRunIDs
}

func validateWaitingCancellationSpawningItem(plan cancellationPlan, item transcript.Item) error {
	switch {
	case item.ID() != plan.target.run.Lineage().SpawnedByItemID:
		return fmt.Errorf(
			"runs: waiting child Run %q resolved spawning Item %q, want %q",
			plan.target.run.ID(),
			item.ID(),
			plan.target.run.Lineage().SpawnedByItemID,
		)
	case item.SessionID() != plan.root.run.SessionID():
		return fmt.Errorf(
			"runs: spawning Item %q belongs to Session %q, want %q",
			item.ID(),
			item.SessionID(),
			plan.root.run.SessionID(),
		)
	case item.RunID() != plan.target.run.Lineage().ParentRunID:
		return fmt.Errorf(
			"runs: spawning Item %q belongs to Run %q, want parent Run %q",
			item.ID(),
			item.RunID(),
			plan.target.run.Lineage().ParentRunID,
		)
	case item.Kind() != transcript.ToolCall:
		return fmt.Errorf("runs: spawning Item %q is not a tool call", item.ID())
	case item.Status() != transcript.ItemRunning:
		return fmt.Errorf(
			"runs: spawning Item %q is in status %s, want running",
			item.ID(),
			item.Status(),
		)
	}
	if _, present := item.ToolInvocation(); !present {
		return fmt.Errorf("runs: spawning Item %q has no tool invocation", item.ID())
	}
	if _, failed := item.Failure(); failed {
		return fmt.Errorf("runs: spawning Item %q already carries a failure", item.ID())
	}
	return nil
}

// pendingSessionID is the Session of a hand-off's root Run, which owns it.
func (c *Coordinator) pendingSessionID(ctx context.Context, pending Pending) (string, error) {
	root, found, err := c.runs.Run(ctx, pending.RootRunID)
	if err != nil {
		return "", fmt.Errorf("runs: read waiting root Run %q: %w", pending.RootRunID, err)
	}
	if !found {
		return "", fmt.Errorf("runs: waiting root Run %q is missing", pending.RootRunID)
	}
	return root.SessionID(), nil
}
