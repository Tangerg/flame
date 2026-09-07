package delivery

import (
	"github.com/Tangerg/flame/runtime/protocol"
)

const (
	SchedulesList   Name = "schedules.list"
	SchedulesCreate Name = "schedules.create"
	SchedulesUpdate Name = "schedules.update"
	SchedulesDelete Name = "schedules.delete"
	SchedulesRunNow Name = "schedules.runNow"
)

func registerSchedules(registry *Registry) {
	registry.Query(MethodMeta{
		Name: SchedulesList, CapabilityRules: requires(protocol.FeatureSchedules),
	}, (*Handler).ListSchedules)

	registry.Command(MethodMeta{
		Name: SchedulesCreate, CapabilityRules: requires(protocol.FeatureSchedules),
	}, (*Handler).CreateSchedule)

	registry.Command(MethodMeta{
		Name: SchedulesUpdate, Errors: []string{protocol.ErrRevisionConflict.Error()},
		CapabilityRules: requires(protocol.FeatureSchedules),
	}, (*Handler).UpdateSchedule)

	registry.CommandAck(MethodMeta{
		Name: SchedulesDelete, CapabilityRules: requires(protocol.FeatureSchedules),
	}, (*Handler).DeleteSchedule)

	registry.Command(MethodMeta{
		Name: SchedulesRunNow, CapabilityRules: requires(protocol.FeatureSchedules),
	}, (*Handler).RunScheduleNow)
}
