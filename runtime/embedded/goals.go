package embedded

import (
	"context"

	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/protocol"
)

// StartGoal starts autonomous Goal pursuit for a Session.
func (r *Runtime) StartGoal(ctx context.Context, request protocol.StartGoalRequest, options CommandOptions) (*protocol.Goal, error) {
	return r.invoke[protocol.StartGoalRequest, *protocol.Goal](ctx, delivery.GoalsStart, request, commandOptions(options))
}

// UpdateGoal revises the current Goal objective.
func (r *Runtime) UpdateGoal(ctx context.Context, request protocol.UpdateGoalRequest, options CommandOptions) (*protocol.Goal, error) {
	return r.invoke[protocol.UpdateGoalRequest, *protocol.Goal](ctx, delivery.GoalsUpdate, request, commandOptions(options))
}

// ClearGoal clears autonomous Goal pursuit.
func (r *Runtime) ClearGoal(ctx context.Context, request protocol.GoalRequest, options CommandOptions) error {
	return r.invokeAck(ctx, delivery.GoalsClear, request, commandOptions(options))
}

// GetGoal returns the Session's current Goal, or nil when none exists.
func (r *Runtime) GetGoal(ctx context.Context, request protocol.GoalRequest, options CallOptions) (*protocol.Goal, error) {
	return r.invoke[protocol.GoalRequest, *protocol.Goal](ctx, delivery.GoalsGet, request, callOptions(options))
}

// StopGoal stops autonomous Goal pursuit.
func (r *Runtime) StopGoal(ctx context.Context, request protocol.GoalRequest, options CommandOptions) (*protocol.Goal, error) {
	return r.invoke[protocol.GoalRequest, *protocol.Goal](ctx, delivery.GoalsStop, request, commandOptions(options))
}

// ResumeGoal resumes paused Goal pursuit.
func (r *Runtime) ResumeGoal(ctx context.Context, request protocol.GoalRequest, options CommandOptions) (*protocol.Goal, error) {
	return r.invoke[protocol.GoalRequest, *protocol.Goal](ctx, delivery.GoalsResume, request, commandOptions(options))
}
