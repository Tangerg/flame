package runtime

import (
	"context"

	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/protocol"
)

// ListSchedules returns one cursor page of schedules.
func (r *Runtime) ListSchedules(ctx context.Context, request protocol.PageQuery, options CallOptions) (*protocol.Page[protocol.Schedule], error) {
	return r.invoke[protocol.PageQuery, *protocol.Page[protocol.Schedule]](ctx, delivery.SchedulesList, request, callOptions(options))
}

// CreateSchedule creates a schedule.
func (r *Runtime) CreateSchedule(ctx context.Context, request protocol.CreateScheduleRequest, options CommandOptions) (*protocol.Schedule, error) {
	return r.invoke[protocol.CreateScheduleRequest, *protocol.Schedule](ctx, delivery.SchedulesCreate, request, commandOptions(options))
}

// UpdateSchedule applies a revision-checked schedule edit.
func (r *Runtime) UpdateSchedule(ctx context.Context, request protocol.UpdateScheduleRequest, options CommandOptions) (*protocol.Schedule, error) {
	return r.invoke[protocol.UpdateScheduleRequest, *protocol.Schedule](ctx, delivery.SchedulesUpdate, request, commandOptions(options))
}

// DeleteSchedule deletes a schedule.
func (r *Runtime) DeleteSchedule(ctx context.Context, request protocol.DeleteScheduleRequest, options CommandOptions) error {
	return r.invokeAck(ctx, delivery.SchedulesDelete, request, commandOptions(options))
}

// RunScheduleNow fires a schedule without advancing its cron cursor.
func (r *Runtime) RunScheduleNow(ctx context.Context, request protocol.RunScheduleNowRequest, options CommandOptions) (*protocol.RunScheduleNowResponse, error) {
	return r.invoke[protocol.RunScheduleNowRequest, *protocol.RunScheduleNowResponse](ctx, delivery.SchedulesRunNow, request, commandOptions(options))
}
