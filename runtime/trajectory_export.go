package runtime

import (
	"context"

	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/protocol"
)

func (r *binding) ExportTrajectory(ctx context.Context, request protocol.ExportTrajectoryRequest, options CallOptions) (*protocol.ExportTrajectoryResponse, error) {
	return r.invoke[protocol.ExportTrajectoryRequest, *protocol.ExportTrajectoryResponse](ctx, delivery.SessionsExportTrajectory, request, callOptions(options))
}
