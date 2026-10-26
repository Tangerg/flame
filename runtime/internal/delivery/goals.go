package delivery

import (
	"context"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

const (
	goalsStart  Name = "goals.start"
	goalsUpdate Name = "goals.update"
	goalsClear  Name = "goals.clear"
	goalsGet    Name = "goals.get"
	goalsStop   Name = "goals.stop"
	goalsResume Name = "goals.resume"
)

func registerGoals(registry *Registry) {
	registry.command(MethodMeta{
		Name: goalsStart, GoName: "StartGoal", Errors: []string{protocol.ErrSessionNotFound.Error()},
		CapabilityRules: requires(protocol.FeatureGoals),
	}, func(service interface {
		StartGoal(context.Context, protocol.StartGoalRequest) (*protocol.Goal, error)
	}, ctx context.Context, request protocol.StartGoalRequest) (*protocol.Goal, error) {
		return service.StartGoal(ctx, request)
	})

	registry.command(MethodMeta{
		Name: goalsUpdate, GoName: "UpdateGoal", Errors: []string{protocol.ErrSessionNotFound.Error()},
		CapabilityRules: requires(protocol.FeatureGoals),
	}, func(service interface {
		UpdateGoal(context.Context, protocol.UpdateGoalRequest) (*protocol.Goal, error)
	}, ctx context.Context, request protocol.UpdateGoalRequest) (*protocol.Goal, error) {
		return service.UpdateGoal(ctx, request)
	})

	registry.commandAck(MethodMeta{
		Name: goalsClear, GoName: "ClearGoal", Errors: []string{protocol.ErrSessionNotFound.Error()},
		CapabilityRules: requires(protocol.FeatureGoals),
	}, func(service interface {
		ClearGoal(context.Context, protocol.GoalRequest) error
	}, ctx context.Context, request protocol.GoalRequest) error {
		return service.ClearGoal(ctx, request)
	})

	registry.query(MethodMeta{
		Name: goalsGet, GoName: "GetGoal", Errors: []string{protocol.ErrSessionNotFound.Error()},
		// A session with no goal is not an error, so the published result admits null.
		ResultNullable: true, CapabilityRules: requires(protocol.FeatureGoals),
	}, func(service interface {
		GetGoal(context.Context, protocol.GoalRequest) (*protocol.Goal, error)
	}, ctx context.Context, request protocol.GoalRequest) (*protocol.Goal, error) {
		return service.GetGoal(ctx, request)
	})

	registry.command(MethodMeta{
		Name: goalsStop, GoName: "StopGoal", Errors: []string{protocol.ErrSessionNotFound.Error()},
		CapabilityRules: requires(protocol.FeatureGoals),
	}, func(service interface {
		StopGoal(context.Context, protocol.GoalRequest) (*protocol.Goal, error)
	}, ctx context.Context, request protocol.GoalRequest) (*protocol.Goal, error) {
		return service.StopGoal(ctx, request)
	})

	registry.command(MethodMeta{
		Name: goalsResume, GoName: "ResumeGoal", Errors: []string{protocol.ErrSessionNotFound.Error()},
		CapabilityRules: requires(protocol.FeatureGoals),
	}, func(service interface {
		ResumeGoal(context.Context, protocol.GoalRequest) (*protocol.Goal, error)
	}, ctx context.Context, request protocol.GoalRequest) (*protocol.Goal, error) {
		return service.ResumeGoal(ctx, request)
	})
}
