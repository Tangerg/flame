package embedded

import (
	"context"

	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/protocol"
)

// ListAgentDocs returns agent instruction documents applicable to a workspace.
func (r *Runtime) ListAgentDocs(ctx context.Context, request protocol.WorkspaceQuery, options CallOptions) (*protocol.Page[protocol.AgentDoc], error) {
	return r.invoke[protocol.WorkspaceQuery, *protocol.Page[protocol.AgentDoc]](ctx, delivery.AgentDocsList, request, callOptions(options))
}
