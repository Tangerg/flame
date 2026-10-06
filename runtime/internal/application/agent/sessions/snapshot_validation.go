package sessions

import (
	"fmt"
	"slices"

	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/domain/run/toolresult"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
)

// ValidateToolResults verifies that every typed transcript offload has exactly
// one matching portable blob and that no blob is detached from its item.
func (s Snapshot) ValidateToolResults() error {
	byItem := make(map[string]toolresult.Blob, len(s.ToolResults))
	byID := make(map[toolresult.ID]string, len(s.ToolResults))
	for index, blob := range s.ToolResults {
		if err := blob.Validate(); err != nil {
			return fmt.Errorf("sessions: tool result %d: %w", index, err)
		}
		if blob.SessionID != s.Session.ID() {
			return fmt.Errorf("sessions: tool result %q belongs to session %q, want %q", blob.ID, blob.SessionID, s.Session.ID())
		}
		if _, duplicate := byItem[blob.ItemID]; duplicate {
			return fmt.Errorf("sessions: multiple tool results are bound to item %q", blob.ItemID)
		}
		if owner, duplicate := byID[blob.ID]; duplicate {
			return fmt.Errorf("sessions: tool result %q is bound to both items %q and %q", blob.ID, owner, blob.ItemID)
		}
		byItem[blob.ItemID] = blob
		byID[blob.ID] = blob.ItemID
	}

	for _, item := range s.Items {
		invocation, present := item.ToolInvocation()
		if !present || invocation.Offload == nil {
			continue
		}
		ref := *invocation.Offload
		if err := ref.Validate(); err != nil {
			return fmt.Errorf("sessions: item %q offload: %w", item.ID(), err)
		}
		if invocation.Result == nil {
			return fmt.Errorf("sessions: item %q offloaded result is absent", item.ID())
		}
		if _, ok := invocation.Result.String(); !ok {
			return fmt.Errorf("sessions: item %q offloaded result is not a string", item.ID())
		}
		blob, exists := byItem[item.ID()]
		if !exists || blob.ID != ref.ID {
			return fmt.Errorf("sessions: item %q references missing tool result %q", item.ID(), ref.ID)
		}
		delete(byItem, item.ID())
	}
	for itemID, blob := range byItem {
		return fmt.Errorf("sessions: tool result %q references missing transcript item %q", blob.ID, itemID)
	}
	return nil
}

// HydratedItems returns the Items with each offloaded result replaced by its
// body, the form transcript reads present. The snapshot is not mutated.
func (s Snapshot) HydratedItems() ([]transcript.Item, error) {
	bodies := make(map[toolresult.ID]string, len(s.ToolResults))
	for _, blob := range s.ToolResults {
		bodies[blob.ID] = blob.Body
	}
	items := slices.Clone(s.Items)
	for i, item := range items {
		snapshot := item.Snapshot()
		if snapshot.Tool == nil || snapshot.Tool.Offload == nil {
			continue
		}
		body, found := bodies[snapshot.Tool.Offload.ID]
		if !found {
			return nil, fmt.Errorf("sessions: item %q references missing tool result %q", item.ID(), snapshot.Tool.Offload.ID)
		}
		result, err := tool.NewResult(body)
		if err != nil {
			return nil, fmt.Errorf("sessions: hydrate item %q: %w", item.ID(), err)
		}
		snapshot.Tool.Result = &result
		hydrated, err := transcript.RestoreItem(snapshot)
		if err != nil {
			return nil, fmt.Errorf("sessions: hydrate item %q: %w", item.ID(), err)
		}
		items[i] = hydrated
	}
	return items, nil
}
