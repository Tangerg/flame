package runs

import (
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
)

func validateRecoveryTranscript(tree recoveryRunTree, items []transcript.Item) error {
	sessionID := tree.root.SessionID()
	seen := make(map[string]int, len(items))
	for index, item := range items {
		if err := item.Validate(); err != nil {
			return fmt.Errorf("runs: validate recovery transcript Item[%d] %q: %w", index, item.ID(), err)
		}
		if item.SessionID() != sessionID {
			return fmt.Errorf(
				"runs: recovery transcript Item[%d] %q belongs to Session %q, want %q",
				index,
				item.ID(),
				item.SessionID(),
				sessionID,
			)
		}
		if first, duplicate := seen[item.ID()]; duplicate {
			return fmt.Errorf(
				"runs: recovery transcript Item[%d] %q duplicates Item[%d] identity",
				index,
				item.ID(),
				first,
			)
		}
		seen[item.ID()] = index
		if item.Status() == transcript.ItemRunning {
			if _, active := tree.runsByID[item.RunID()]; !active {
				return fmt.Errorf(
					"runs: recovery transcript Running Item[%d] %q has no owner in active tree %q",
					index,
					item.ID(),
					tree.root.ID(),
				)
			}
		}
	}
	return nil
}

func validateRecoveryToolInvocationItems(
	tree recoveryRunTree,
	items []transcript.Item,
	invocations []OpenToolInvocation,
) error {
	itemsByID := make(map[string]transcript.Item, len(items))
	for _, item := range items {
		itemsByID[item.ID()] = item
	}
	for index, invocation := range invocations {
		if invocation.SessionID != tree.root.SessionID() {
			continue
		}
		if _, active := tree.runsByID[invocation.RunID]; !active {
			// The invocation journal may outlive an already-terminal Run. It no
			// longer has an active Transcript lifecycle to reconcile.
			continue
		}
		item, present := itemsByID[invocation.ItemID]
		if !present {
			return fmt.Errorf(
				"runs: open Tool invocation[%d] %q has no recovery transcript Item %q",
				index,
				invocation.CallID,
				invocation.ItemID,
			)
		}
		if item.RunID() != invocation.RunID {
			return fmt.Errorf(
				"runs: open Tool invocation[%d] %q Item %q belongs to Run %q, want %q",
				index,
				invocation.CallID,
				item.ID(),
				item.RunID(),
				invocation.RunID,
			)
		}
		if item.Kind() != transcript.ToolCall || item.Status() != transcript.ItemRunning {
			return fmt.Errorf(
				"runs: open Tool invocation[%d] %q Item %q is not a Running ToolCall",
				index,
				invocation.CallID,
				item.ID(),
			)
		}
	}
	return nil
}
