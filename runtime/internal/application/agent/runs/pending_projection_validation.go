package runs

import (
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
)

func indexPendingItems(items []transcript.Item) map[string]transcript.Item {
	indexed := make(map[string]transcript.Item, len(items))
	for _, item := range items {
		indexed[item.ID()] = item
	}
	return indexed
}

func validatePendingRunningItems(
	rootRunID string,
	activeRuns map[string]run.Run,
	items []transcript.Item,
	interruptItems map[string]struct{},
	drainedItems map[string]struct{},
) error {
	for _, item := range items {
		if _, active := activeRuns[item.RunID()]; !active || item.Status() != transcript.ItemRunning {
			continue
		}
		_, belongsToInterrupt := interruptItems[item.ID()]
		_, belongsToDrainedTool := drainedItems[item.ID()]
		if !belongsToInterrupt && !belongsToDrainedTool {
			return fmt.Errorf(
				"runs: validate parked Run tree %q: Running Item %q in Run %q has no matching interrupt or drained Tool",
				rootRunID,
				item.ID(),
				item.RunID(),
			)
		}
	}
	return nil
}

func validatePendingContinuationTools(
	rootRunID string,
	continuation Continuation,
	itemsByID map[string]transcript.Item,
	claimedItems map[string]string,
) error {
	for _, drained := range continuation.DrainedTools {
		if err := claimPendingItem(rootRunID, continuation.RunID, drained.ItemID, "drained tool", claimedItems); err != nil {
			return err
		}
		if _, _, err := drainedToolItem(itemsByID, continuation.RunID, drained); err != nil {
			return fmt.Errorf("runs: validate parked Run tree %q: %w", rootRunID, err)
		}
	}
	return nil
}

func claimPendingItem(
	rootRunID string,
	runID string,
	itemID string,
	role string,
	claimedItems map[string]string,
) error {
	if previous, duplicate := claimedItems[itemID]; duplicate {
		return fmt.Errorf(
			"runs: validate parked Run tree %q: Item %q in Run %q is both %s and %s",
			rootRunID,
			itemID,
			runID,
			previous,
			role,
		)
	}
	claimedItems[itemID] = role
	return nil
}
