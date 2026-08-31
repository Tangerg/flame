package runtime

import (
	"context"

	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/protocol"
)

// ListRecipes returns recipes applicable to a workspace.
func (r *Runtime) ListRecipes(ctx context.Context, request protocol.WorkspaceQuery, options CallOptions) (*protocol.Page[protocol.Recipe], error) {
	return r.invoke[protocol.WorkspaceQuery, *protocol.Page[protocol.Recipe]](ctx, delivery.RecipesList, request, callOptions(options))
}
