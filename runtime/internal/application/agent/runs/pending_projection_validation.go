package runs

import (
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
)

func indexPendingItems(items []transcript.Item) map[identity.ItemID]transcript.Item {
	indexed := make(map[identity.ItemID]transcript.Item, len(items))
	for _, item := range items {
		indexed[item.ID()] = item
	}
	return indexed
}

func validatePendingRunningItems(
	rootRunID identity.RunID,
	activeRuns map[identity.RunID]run.Run,
	items []transcript.Item,
	interruptItems map[identity.ItemID]struct{},
	drainedItems map[identity.ItemID]struct{},
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
	rootRunID identity.RunID,
	continuation Continuation,
	itemsByID map[identity.ItemID]transcript.Item,
	claimedItems map[identity.ItemID]string,
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
	rootRunID identity.RunID,
	runID identity.RunID,
	itemID identity.ItemID,
	role string,
	claimedItems map[identity.ItemID]string,
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
