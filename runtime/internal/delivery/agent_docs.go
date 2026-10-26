package delivery

import (
	"context"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

const agentDocsList Name = "agentDocs.list"

func registerAgentDocs(registry *Registry) {
	registry.query(MethodMeta{
		Name: agentDocsList, GoName: "ListAgentDocs",
		Errors: []string{protocol.ErrWorkspaceUnavailable.Error(), protocol.ErrPromptSourceTooLarge.Error()},
	}, func(service interface {
		ListAgentDocs(context.Context, protocol.WorkspaceQuery) (*protocol.Page[protocol.AgentDoc], error)
	}, ctx context.Context, request protocol.WorkspaceQuery) (*protocol.Page[protocol.AgentDoc], error) {
		return service.ListAgentDocs(ctx, request)
	})
}
