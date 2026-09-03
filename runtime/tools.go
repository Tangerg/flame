package runtime

import (
	"context"

	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/protocol"
)

// ListTools returns the safe direct-invocation tool catalog in unique name order.
func (r *Runtime) ListTools(ctx context.Context, options CallOptions) (*protocol.Page[protocol.ToolSpec], error) {
	return r.invoke[struct{}, *protocol.Page[protocol.ToolSpec]](ctx, delivery.ToolsList, struct{}{}, callOptions(options))
}

// InvokeTool invokes one Runtime tool outside an Agent Run.
func (r *Runtime) InvokeTool(ctx context.Context, request protocol.InvokeToolRequest, options CommandOptions) (any, error) {
	return r.invoke[protocol.InvokeToolRequest, any](ctx, delivery.ToolsInvoke, request, commandOptions(options))
}
