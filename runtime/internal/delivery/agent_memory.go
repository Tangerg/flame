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
	registry.query(MethodMeta{
		Name: AgentMemoryList, CapabilityRules: requires(protocol.FeatureAgentMemory),
	}, (*Handler).ListAgentMemory)

	registry.commandAck(MethodMeta{
		Name: AgentMemoryReview, CapabilityRules: requires(protocol.FeatureAgentMemory),
	}, (*Handler).ReviewAgentMemory)

	registry.command(MethodMeta{
		Name: AgentMemoryUpdate, CapabilityRules: requires(protocol.FeatureAgentMemory),
	}, (*Handler).UpdateAgentMemory)

	registry.commandAck(MethodMeta{
		Name: AgentMemoryDelete, CapabilityRules: requires(protocol.FeatureAgentMemory),
	}, (*Handler).DeleteAgentMemory)

	registry.command(MethodMeta{
		Name: AgentMemoryAdd, CapabilityRules: requires(protocol.FeatureAgentMemory),
	}, (*Handler).AddAgentMemory)
}
