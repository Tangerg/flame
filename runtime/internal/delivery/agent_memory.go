package delivery

import (
	"context"

	"github.com/Tangerg/flame/runtime/protocol"
)

const (
	AgentMemoryReview Name = "agentMemory.review"
	AgentMemoryUpdate Name = "agentMemory.update"
	AgentMemoryAdd    Name = "agentMemory.add"
)

func registerAgentMemory(registry *Registry) {

	registry.commandAck(MethodMeta{Idempotency: IdempotencyOwnerResponse,
		Name: AgentMemoryReview, Errors: []string{protocol.ErrPluginUnavailable.Error()}, CapabilityRules: requires(protocol.FeatureAgentMemory),
	}, func(service interface {
		ReviewAgentMemory(context.Context, protocol.AgentMemoryReviewRequest) error
	}, ctx context.Context, request protocol.AgentMemoryReviewRequest) error {
		return service.ReviewAgentMemory(ctx, request)
	})

	registry.command(MethodMeta{Idempotency: IdempotencyOwnerResponse,
		Name: AgentMemoryUpdate, Errors: []string{protocol.ErrPluginUnavailable.Error()}, CapabilityRules: requires(protocol.FeatureAgentMemory),
	}, func(service interface {
		UpdateAgentMemory(context.Context, protocol.AgentMemoryUpdateRequest) (*protocol.AgentMemoryItem, error)
	}, ctx context.Context, request protocol.AgentMemoryUpdateRequest) (*protocol.AgentMemoryItem, error) {
		return service.UpdateAgentMemory(ctx, request)
	})

	registry.command(MethodMeta{Idempotency: IdempotencyOwnerResponse,
		Name: AgentMemoryAdd, Errors: []string{protocol.ErrPluginUnavailable.Error()}, CapabilityRules: requires(protocol.FeatureAgentMemory),
	}, func(service interface {
		AddAgentMemory(context.Context, protocol.AgentMemoryAddRequest) (*protocol.AgentMemoryItem, error)
	}, ctx context.Context, request protocol.AgentMemoryAddRequest) (*protocol.AgentMemoryItem, error) {
		return service.AddAgentMemory(ctx, request)
	})
}
