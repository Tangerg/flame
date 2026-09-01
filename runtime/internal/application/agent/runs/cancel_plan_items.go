package runs

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/domain/run/interrupt"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
)

func (c *Coordinator) loadWaitingCancellationItems(ctx context.Context, plan *cancellationPlan) error {
	if plan == nil {
		return errors.New("runs: waiting cancellation plan is required")
	}
	if err := c.loadWaitingCancellationSpawningItem(ctx, plan); err != nil {
		return err
	}
	targetRunIDs := cancellationTargetRunIDs(plan.targetSubtree)
	seenToolItems, err := c.loadWaitingCancellationInterruptItems(ctx, plan, targetRunIDs)
	if err != nil {
		return err
	}
	return c.loadWaitingCancellationDrainedItems(ctx, plan, targetRunIDs, seenToolItems)
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

func (c *Coordinator) loadWaitingCancellationInterruptItems(
	ctx context.Context,
	plan *cancellationPlan,
	targetRunIDs map[string]struct{},
) (map[string]struct{}, error) {
	seenToolItems := make(map[string]struct{})
	for _, request := range plan.pending.Interrupts {
		if _, targeted := targetRunIDs[request.RunID]; !targeted {
			continue
		}
		item, found, err := c.items.Item(ctx, request.ItemID)
		if err != nil {
			return nil, fmt.Errorf(
				"runs: read waiting interrupt Item %q for Run %q: %w",
				request.ItemID,
				request.RunID,
				err,
			)
		}
		if !found {
			return nil, fmt.Errorf(
				"runs: waiting interrupt Item %q for Run %q is missing",
				request.ItemID,
				request.RunID,
			)
		}
		if err := validateWaitingCancellationInterruptItem(*plan, request, item); err != nil {
			return nil, err
		}
		plan.targetInterruptItems = append(plan.targetInterruptItems, item)
		if item.Kind() == transcript.ToolCall {
			seenToolItems[item.ID()] = struct{}{}
		}
	}
	return seenToolItems, nil
}

func (c *Coordinator) loadWaitingCancellationDrainedItems(
	ctx context.Context,
	plan *cancellationPlan,
	targetRunIDs map[string]struct{},
	seenToolItems map[string]struct{},
) error {
	for _, continuation := range plan.pending.Continuations {
		if _, targeted := targetRunIDs[continuation.RunID]; !targeted {
			continue
		}
		for _, drained := range continuation.DrainedTools {
			if _, duplicate := seenToolItems[drained.ItemID]; duplicate {
				return fmt.Errorf(
					"runs: waiting child cancellation Tool Item %q is both an interrupt and a drained tool",
					drained.ItemID,
				)
			}
			item, found, err := c.items.Item(ctx, drained.ItemID)
			if err != nil {
				return fmt.Errorf("runs: read waiting drained Tool Item %q: %w", drained.ItemID, err)
			}
			if !found {
				return fmt.Errorf("runs: waiting drained Tool Item %q is missing", drained.ItemID)
			}
			if err := validateWaitingCancellationDrainedItem(*plan, continuation, drained, item); err != nil {
				return err
			}
			seenToolItems[item.ID()] = struct{}{}
			plan.targetDrainedItems = append(plan.targetDrainedItems, item)
		}
	}
	return nil
}

func validateWaitingCancellationDrainedItem(
	plan cancellationPlan,
	continuation Continuation,
	drained DrainedTool,
	item transcript.Item,
) error {
	invocation, present := item.ToolInvocation()
	if item.ID() != drained.ItemID || item.SessionID() != plan.root.run.SessionID() ||
		item.RunID() != continuation.RunID || item.Kind() != transcript.ToolCall ||
		item.Status() != transcript.ItemRunning || !present ||
		invocation.Name != drained.Name || invocation.Arguments.Canonical() != drained.Arguments {
		return fmt.Errorf(
			"runs: waiting drained Tool Item %q differs from Run %q continuation",
			drained.ItemID,
			continuation.RunID,
		)
	}
	if _, failed := item.Failure(); failed {
		return fmt.Errorf("runs: waiting drained Tool Item %q already carries a failure", item.ID())
	}
	return nil
}

func validateWaitingCancellationInterruptItem(
	plan cancellationPlan,
	request transcript.Interrupt,
	item transcript.Item,
) error {
	switch {
	case item.ID() != request.ItemID:
		return fmt.Errorf(
			"runs: waiting interrupt for Run %q resolved Item %q, want %q",
			request.RunID,
			item.ID(),
			request.ItemID,
		)
	case item.SessionID() != plan.root.run.SessionID():
		return fmt.Errorf(
			"runs: waiting interrupt Item %q belongs to Session %q, want %q",
			item.ID(),
			item.SessionID(),
			plan.root.run.SessionID(),
		)
	case item.RunID() != request.RunID:
		return fmt.Errorf(
			"runs: waiting interrupt Item %q belongs to Run %q, want %q",
			item.ID(),
			item.RunID(),
			request.RunID,
		)
	}
	switch request.Kind {
	case interrupt.Question:
		question, present := item.Question()
		if item.Kind() != transcript.QuestionItem || item.Status() != transcript.ItemCompleted ||
			!present ||
			request.Question == nil ||
			!question.Equal(*request.Question) {
			return fmt.Errorf(
				"runs: waiting question Item %q differs from its interrupt",
				item.ID(),
			)
		}
	case interrupt.Approval:
		invocation, present := item.ToolInvocation()
		if item.Kind() != transcript.ToolCall || item.Status() != transcript.ItemRunning ||
			!present ||
			request.Approval == nil ||
			!invocation.Equal(request.Approval.Tool) {
			return fmt.Errorf(
				"runs: waiting approval Item %q differs from its interrupt",
				item.ID(),
			)
		}
	default:
		return fmt.Errorf(
			"runs: waiting interrupt Item %q has unsupported kind %s",
			item.ID(),
			request.Kind,
		)
	}
	return nil
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
