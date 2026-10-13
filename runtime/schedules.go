package runtime

import (
	"context"

	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/protocol"
)

// UpdateSchedule applies a revision-checked schedule edit.
func (r *binding) UpdateSchedule(ctx context.Context, request protocol.UpdateScheduleRequest, options CommandOptions) (*protocol.Schedule, error) {
	return r.invoke[protocol.UpdateScheduleRequest, *protocol.Schedule](ctx, delivery.SchedulesUpdate, request, commandOptions(options))
}
