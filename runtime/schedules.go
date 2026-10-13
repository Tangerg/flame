package runtime

import (
	"context"

	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/protocol"
)

// CreateSchedule creates a schedule.
func (r *binding) CreateSchedule(ctx context.Context, request protocol.CreateScheduleRequest, options CommandOptions) (*protocol.Schedule, error) {
	return r.invoke[protocol.CreateScheduleRequest, *protocol.Schedule](ctx, delivery.SchedulesCreate, request, commandOptions(options))
}

// UpdateSchedule applies a revision-checked schedule edit.
func (r *binding) UpdateSchedule(ctx context.Context, request protocol.UpdateScheduleRequest, options CommandOptions) (*protocol.Schedule, error) {
	return r.invoke[protocol.UpdateScheduleRequest, *protocol.Schedule](ctx, delivery.SchedulesUpdate, request, commandOptions(options))
}

// RunScheduleNow fires a schedule without advancing its cron cursor.
func (r *binding) RunScheduleNow(ctx context.Context, request protocol.RunScheduleNowRequest, options CommandOptions) (*protocol.RunScheduleNowResponse, error) {
	return r.invoke[protocol.RunScheduleNowRequest, *protocol.RunScheduleNowResponse](ctx, delivery.SchedulesRunNow, request, commandOptions(options))
}
