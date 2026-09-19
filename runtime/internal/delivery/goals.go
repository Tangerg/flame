package delivery

import (
	"github.com/Tangerg/flame/runtime/protocol"
)

const (
	GoalsStart  Name = "goals.start"
	GoalsUpdate Name = "goals.update"
	GoalsClear  Name = "goals.clear"
	GoalsGet    Name = "goals.get"
	GoalsStop   Name = "goals.stop"
	GoalsResume Name = "goals.resume"
)

func registerGoals(registry *Registry) {
	registry.command(MethodMeta{
		Name: GoalsStart, Errors: []string{protocol.ErrSessionNotFound.Error()},
		CapabilityRules: requires(protocol.FeatureGoals),
	}, (*Handler).StartGoal)

	registry.command(MethodMeta{
		Name: GoalsUpdate, Errors: []string{protocol.ErrSessionNotFound.Error()},
		CapabilityRules: requires(protocol.FeatureGoals),
	}, (*Handler).UpdateGoal)

	registry.commandAck(MethodMeta{
		Name: GoalsClear, Errors: []string{protocol.ErrSessionNotFound.Error()},
		CapabilityRules: requires(protocol.FeatureGoals),
	}, (*Handler).ClearGoal)

	registry.query(MethodMeta{
		Name: GoalsGet, Errors: []string{protocol.ErrSessionNotFound.Error()},
		// A session with no goal is not an error, so the published result admits null.
		ResultNullable: true, CapabilityRules: requires(protocol.FeatureGoals),
	}, (*Handler).GetGoal)

	registry.command(MethodMeta{
		Name: GoalsStop, Errors: []string{protocol.ErrSessionNotFound.Error()},
		CapabilityRules: requires(protocol.FeatureGoals),
	}, (*Handler).StopGoal)

	registry.command(MethodMeta{
		Name: GoalsResume, Errors: []string{protocol.ErrSessionNotFound.Error()},
		CapabilityRules: requires(protocol.FeatureGoals),
	}, (*Handler).ResumeGoal)
}
