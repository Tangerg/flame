package sessions

import (
	"fmt"
	"slices"

	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/domain/run/toolresult"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
)

// ValidateToolResults verifies that every typed transcript offload names
// exactly one blob of this Session and that every blob is named by one Item.
func (s Snapshot) ValidateToolResults() error {
	owners := make(map[toolresult.ID]string, len(s.ToolResults))
	for index, blob := range s.ToolResults {
		if err := blob.Validate(); err != nil {
			return fmt.Errorf("sessions: tool result %d: %w", index, err)
		}
		if blob.SessionID != s.Session.ID() {
			return fmt.Errorf("sessions: tool result %q belongs to session %q, want %q", blob.ID, blob.SessionID, s.Session.ID())
		}
		if _, duplicate := owners[blob.ID]; duplicate {
			return fmt.Errorf("sessions: tool result %q appears more than once", blob.ID)
		}
		owners[blob.ID] = ""
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
		owner, exists := owners[ref.ID]
		if !exists {
			return fmt.Errorf("sessions: item %q references missing tool result %q", item.ID(), ref.ID)
		}
		if owner != "" {
			return fmt.Errorf("sessions: tool result %q is named by both items %q and %q", ref.ID, owner, item.ID())
		}
		owners[ref.ID] = item.ID()
	}
	for id, owner := range owners {
		if owner == "" {
			return fmt.Errorf("sessions: tool result %q is named by no transcript item", id)
		}
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
