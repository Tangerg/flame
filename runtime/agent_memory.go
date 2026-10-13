package runtime

import (
	"context"

	"github.com/Tangerg/flame/runtime/internal/delivery"
	agentmemory "github.com/Tangerg/flame/runtime/internal/plugins/memory/domain"
	"github.com/Tangerg/flame/runtime/protocol"
)

// MaximumAgentMemoryItemsPerTarget projects the domain-owned target capacity for
// complete Go collectors; it does not establish a separate management limit.
const MaximumAgentMemoryItemsPerTarget = agentmemory.MaxVisiblePerTarget

// AddAgentMemory adds one curated Agent memory item.
func (r *binding) AddAgentMemory(ctx context.Context, request protocol.AgentMemoryAddRequest, options CommandOptions) (*protocol.AgentMemoryItem, error) {
	return r.invoke[protocol.AgentMemoryAddRequest, *protocol.AgentMemoryItem](ctx, delivery.AgentMemoryAdd, request, commandOptions(options))
}
