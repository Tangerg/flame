package embedded

import (
	"context"

	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/protocol"
)

// Discover returns the protocol range and capabilities of this Runtime.
func (r *Runtime) Discover(ctx context.Context, options CallOptions) (*protocol.DiscoverResponse, error) {
	return r.invoke[struct{}, *protocol.DiscoverResponse](ctx, delivery.RuntimeDiscover, struct{}{}, callOptions(options))
}
