package runtime

import (
	"context"

	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/protocol"
)

func (r *binding) ListSessionTrajectory(ctx context.Context, request protocol.ListSessionTrajectoryRequest, options CallOptions) (*protocol.Page[protocol.TrajectoryEntry], error) {
	return r.invoke[protocol.ListSessionTrajectoryRequest, *protocol.Page[protocol.TrajectoryEntry]](ctx, delivery.SessionsTrajectory, request, callOptions(options))
}
