package delivery

import (
	"context"

	"github.com/Tangerg/flame/runtime/protocol"
)

const (
	SchedulesUpdate Name = "schedules.update"
)

func registerSchedules(registry *Registry) {

	registry.command(MethodMeta{Idempotency: IdempotencyOwnerResponse,
		Name: SchedulesUpdate, Errors: []string{protocol.ErrPluginUnavailable.Error(), protocol.ErrScheduleNotFound.Error(), protocol.ErrRevisionConflict.Error(), protocol.ErrWorkspaceUnavailable.Error()},
		CapabilityRules: requires(protocol.FeatureSchedules),
	}, func(service interface {
		UpdateSchedule(context.Context, protocol.UpdateScheduleRequest) (*protocol.Schedule, error)
	}, ctx context.Context, request protocol.UpdateScheduleRequest) (*protocol.Schedule, error) {
		return service.UpdateSchedule(ctx, request)
	})

}
