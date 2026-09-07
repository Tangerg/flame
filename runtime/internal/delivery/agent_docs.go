package delivery

import (
	"github.com/Tangerg/flame/runtime/protocol"
)

const AgentDocsList Name = "agentDocs.list"

func registerAgentDocs(registry *Registry) {
	registry.Query(MethodMeta{
		Name:   AgentDocsList,
		Errors: []string{protocol.ErrWorkspaceUnavailable.Error()},
	}, (*Handler).ListAgentDocs)
}
