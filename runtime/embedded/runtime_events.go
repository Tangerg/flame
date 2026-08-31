package embedded

import (
	"context"
	"iter"

	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/protocol"
)

// SubscribeRuntime observes Runtime-wide change topics and file watches.
func (r *Runtime) SubscribeRuntime(ctx context.Context, request protocol.RuntimeSubscribeRequest, options SubscriptionOptions) (*protocol.RuntimeSubscribeResponse, iter.Seq2[protocol.RuntimeEvent, error], error) {
	return r.invokeStream[protocol.RuntimeSubscribeRequest, *protocol.RuntimeSubscribeResponse, protocol.RuntimeEvent](ctx, delivery.RuntimeSubscribe, request, subscriptionOptions(options))
}
