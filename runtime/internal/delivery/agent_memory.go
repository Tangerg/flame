package delivery

import (
	"github.com/Tangerg/flame/runtime/protocol"
)

const (
	AgentMemoryList   Name = "agentMemory.list"
	AgentMemoryReview Name = "agentMemory.review"
	AgentMemoryUpdate Name = "agentMemory.update"
	AgentMemoryDelete Name = "agentMemory.delete"
	AgentMemoryAdd    Name = "agentMemory.add"
)

func registerAgentMemory(registry *Registry) {
	registry.Query(MethodMeta{
		Name: AgentMemoryList, CapabilityRules: requires(protocol.FeatureAgentMemory),
	}, (*Handler).ListAgentMemory)

	registry.CommandAck(MethodMeta{
		Name: AgentMemoryReview, CapabilityRules: requires(protocol.FeatureAgentMemory),
	}, (*Handler).ReviewAgentMemory)

	registry.Command(MethodMeta{
		Name: AgentMemoryUpdate, CapabilityRules: requires(protocol.FeatureAgentMemory),
	}, (*Handler).UpdateAgentMemory)

	registry.CommandAck(MethodMeta{
		Name: AgentMemoryDelete, CapabilityRules: requires(protocol.FeatureAgentMemory),
	}, (*Handler).DeleteAgentMemory)

	registry.Command(MethodMeta{
		Name: AgentMemoryAdd, CapabilityRules: requires(protocol.FeatureAgentMemory),
	}, (*Handler).AddAgentMemory)
}
