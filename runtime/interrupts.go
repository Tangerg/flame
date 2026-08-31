package runtime

import (
	"context"

	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/protocol"
)

// ListInterrupts returns waiting interrupt sets for Run trees.
func (r *Runtime) ListInterrupts(ctx context.Context, request protocol.ListInterruptsRequest, options CallOptions) (*protocol.Page[protocol.PendingInterruptSet], error) {
	return r.invoke[protocol.ListInterruptsRequest, *protocol.Page[protocol.PendingInterruptSet]](ctx, delivery.InterruptsList, request, callOptions(options))
}
