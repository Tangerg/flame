package runtime

import (
	"context"

	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/protocol"
)

// StartGoal starts autonomous Goal pursuit for a Session.
func (r *binding) StartGoal(ctx context.Context, request protocol.StartGoalRequest, options CommandOptions) (*protocol.Goal, error) {
	return r.invoke[protocol.StartGoalRequest, *protocol.Goal](ctx, delivery.GoalsStart, request, commandOptions(options))
}

// UpdateGoal revises the current Goal objective.
func (r *binding) UpdateGoal(ctx context.Context, request protocol.UpdateGoalRequest, options CommandOptions) (*protocol.Goal, error) {
	return r.invoke[protocol.UpdateGoalRequest, *protocol.Goal](ctx, delivery.GoalsUpdate, request, commandOptions(options))
}

// ClearGoal clears autonomous Goal pursuit.
func (r *binding) ClearGoal(ctx context.Context, request protocol.GoalRequest, options CommandOptions) error {
	return r.invokeAck(ctx, delivery.GoalsClear, request, commandOptions(options))
}

// GetGoal returns the Session's current Goal, or nil when none exists.
func (r *binding) GetGoal(ctx context.Context, request protocol.GoalRequest, options CallOptions) (*protocol.Goal, error) {
	return r.invoke[protocol.GoalRequest, *protocol.Goal](ctx, delivery.GoalsGet, request, callOptions(options))
}

// StopGoal stops autonomous Goal pursuit.
func (r *binding) StopGoal(ctx context.Context, request protocol.GoalRequest, options CommandOptions) (*protocol.Goal, error) {
	return r.invoke[protocol.GoalRequest, *protocol.Goal](ctx, delivery.GoalsStop, request, commandOptions(options))
}

// ResumeGoal resumes paused Goal pursuit.
func (r *binding) ResumeGoal(ctx context.Context, request protocol.GoalRequest, options CommandOptions) (*protocol.Goal, error) {
	return r.invoke[protocol.GoalRequest, *protocol.Goal](ctx, delivery.GoalsResume, request, commandOptions(options))
}
