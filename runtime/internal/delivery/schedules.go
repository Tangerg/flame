package delivery

import (
	"context"

	"github.com/Tangerg/flame/runtime/protocol"
)

const (
	SchedulesCreate Name = "schedules.create"
	SchedulesUpdate Name = "schedules.update"
)

func registerSchedules(registry *Registry) {

	registry.command(MethodMeta{Idempotency: IdempotencyOwnerResponse,
		Name: SchedulesCreate, CapabilityRules: requires(protocol.FeatureSchedules),
		Errors: []string{protocol.ErrPluginUnavailable.Error(), protocol.ErrWorkspaceUnavailable.Error()},
	}, func(service interface {
		CreateSchedule(context.Context, protocol.CreateScheduleRequest) (*protocol.Schedule, error)
	}, ctx context.Context, request protocol.CreateScheduleRequest) (*protocol.Schedule, error) {
		return service.CreateSchedule(ctx, request)
	})

	registry.command(MethodMeta{Idempotency: IdempotencyOwnerResponse,
		Name: SchedulesUpdate, Errors: []string{protocol.ErrPluginUnavailable.Error(), protocol.ErrScheduleNotFound.Error(), protocol.ErrRevisionConflict.Error(), protocol.ErrWorkspaceUnavailable.Error()},
		CapabilityRules: requires(protocol.FeatureSchedules),
	}, func(service interface {
		UpdateSchedule(context.Context, protocol.UpdateScheduleRequest) (*protocol.Schedule, error)
	}, ctx context.Context, request protocol.UpdateScheduleRequest) (*protocol.Schedule, error) {
		return service.UpdateSchedule(ctx, request)
	})

}
