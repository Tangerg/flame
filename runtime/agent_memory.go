package runtime

import (
	"context"

	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/protocol"
)

// ListAgentMemory returns curated Agent memory and review candidates.
func (r *binding) ListAgentMemory(ctx context.Context, request protocol.AgentMemoryListRequest, options CallOptions) (*protocol.AgentMemoryList, error) {
	return r.invoke[protocol.AgentMemoryListRequest, *protocol.AgentMemoryList](ctx, delivery.AgentMemoryList, request, callOptions(options))
}

// ReviewAgentMemory accepts or rejects an Agent memory candidate.
func (r *binding) ReviewAgentMemory(ctx context.Context, request protocol.AgentMemoryReviewRequest, options CommandOptions) error {
	return r.invokeAck(ctx, delivery.AgentMemoryReview, request, commandOptions(options))
}

// UpdateAgentMemory updates one curated Agent memory item.
func (r *binding) UpdateAgentMemory(ctx context.Context, request protocol.AgentMemoryUpdateRequest, options CommandOptions) (*protocol.AgentMemoryItem, error) {
	return r.invoke[protocol.AgentMemoryUpdateRequest, *protocol.AgentMemoryItem](ctx, delivery.AgentMemoryUpdate, request, commandOptions(options))
}

// DeleteAgentMemory deletes one curated Agent memory item.
func (r *binding) DeleteAgentMemory(ctx context.Context, request protocol.AgentMemoryItemRequest, options CommandOptions) error {
	return r.invokeAck(ctx, delivery.AgentMemoryDelete, request, commandOptions(options))
}

// AddAgentMemory adds one curated Agent memory item.
func (r *binding) AddAgentMemory(ctx context.Context, request protocol.AgentMemoryAddRequest, options CommandOptions) (*protocol.AgentMemoryItem, error) {
	return r.invoke[protocol.AgentMemoryAddRequest, *protocol.AgentMemoryItem](ctx, delivery.AgentMemoryAdd, request, commandOptions(options))
}
