package delivery

import (
	"context"

	"github.com/Tangerg/flame/runtime/protocol"
)

const (
	AgentMemoryAdd Name = "agentMemory.add"
)

func registerAgentMemory(registry *Registry) {

	registry.command(MethodMeta{Idempotency: IdempotencyOwnerResponse,
		Name: AgentMemoryAdd, Errors: []string{protocol.ErrPluginUnavailable.Error()}, CapabilityRules: requires(protocol.FeatureAgentMemory),
	}, func(service interface {
		AddAgentMemory(context.Context, protocol.AgentMemoryAddRequest) (*protocol.AgentMemoryItem, error)
	}, ctx context.Context, request protocol.AgentMemoryAddRequest) (*protocol.AgentMemoryItem, error) {
		return service.AddAgentMemory(ctx, request)
	})
}
