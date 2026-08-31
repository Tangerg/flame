package delivery

import (
	"context"

	"github.com/Tangerg/flame/runtime/protocol"
)

const PlanGet Name = "plan.get"

func registerPlan(registry *Registry) {
	// The Plan's cold read. A Session with no committed replacement answers with
	// an explicit unwritten value; only a Session that does not exist is an error.
	registry.Query(MethodMeta{
		Name:            PlanGet,
		Errors:          []string{protocol.ErrSessionNotFound.Error()},
		CapabilityRules: requires(protocol.FeaturePlan),
	}, func(service interface {
		GetPlan(context.Context, protocol.GetPlanRequest) (*protocol.Plan, error)
	}, ctx context.Context, request protocol.GetPlanRequest) (*protocol.Plan, error) {
		return service.GetPlan(ctx, request)
	})
}
