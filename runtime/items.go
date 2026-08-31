package runtime

import (
	"context"

	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/protocol"
)

// ListItems returns the authoritative transcript Items for a Session or Run scope.
func (r *Runtime) ListItems(ctx context.Context, request protocol.ListItemsRequest, options CallOptions) (*protocol.ListItemsResponse, error) {
	return r.invoke[protocol.ListItemsRequest, *protocol.ListItemsResponse](ctx, delivery.ItemsList, request, callOptions(options))
}
