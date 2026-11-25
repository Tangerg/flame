package runtimebinding

import (
	"context"
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

type goalBinding interface {
	GetGoal(context.Context, protocol.GoalRequest, protocol.CallOptions) (*protocol.Goal, error)
	StartGoal(context.Context, protocol.StartGoalRequest, protocol.CommandOptions) (*protocol.Goal, error)
	UpdateGoal(context.Context, protocol.UpdateGoalRequest, protocol.CommandOptions) (*protocol.Goal, error)
	ClearGoal(context.Context, protocol.GoalRequest, protocol.CommandOptions) error
	StopGoal(context.Context, protocol.GoalRequest, protocol.CommandOptions) error
	ResumeGoal(context.Context, protocol.GoalRequest, protocol.CommandOptions) (*protocol.Goal, error)
}

func (r *Connection) UpdateGoal(ctx context.Context, update protocol.UpdateGoalRequest) (protocol.Goal, error) {
	if err := protocol.ValidateWireTree(update); err != nil {
		return protocol.Goal{}, fmt.Errorf("update goal: %w", err)
	}
	options := r.commandOptions()
	result, err := r.goals.UpdateGoal(ctx, update, options)
	return goalResult("update goal", update.SessionID, result, err)
}

func (r *Connection) ClearGoal(ctx context.Context, sessionID string) error {
	return r.acknowledgeGoalChange(ctx, "clear goal", sessionID, r.goals.ClearGoal)
}

func (r *Connection) StopGoal(ctx context.Context, sessionID string) error {
	return r.acknowledgeGoalChange(ctx, "stop goal", sessionID, r.goals.StopGoal)
}

func (r *Connection) acknowledgeGoalChange(
	ctx context.Context, operation, sessionID string,
	command func(context.Context, protocol.GoalRequest, protocol.CommandOptions) error,
) error {
	request, err := newGoalRequest(operation, sessionID)
	if err != nil {
		return err
	}
	return classifyError(command(ctx, request, r.commandOptions()))
}

func (r *Connection) GetGoal(ctx context.Context, sessionID string) (protocol.Goal, bool, error) {
	request, err := newGoalRequest("get goal", sessionID)
	if err != nil {
		return protocol.Goal{}, false, err
	}
	result, err := r.goals.GetGoal(ctx, request, r.callOptions())
	if err != nil {
		return protocol.Goal{}, false, classifyError(err)
	}
	if result == nil {
		return protocol.Goal{}, false, nil
	}
	goal, err := goalResult("get goal", sessionID, result, nil)
	return goal, err == nil, err
}

func (r *Connection) StartGoal(ctx context.Context, start protocol.StartGoalRequest) (protocol.Goal, error) {
	if err := protocol.ValidateWireTree(start); err != nil {
		return protocol.Goal{}, fmt.Errorf("start goal: %w", err)
	}
	options := r.commandOptions()
	result, err := r.goals.StartGoal(ctx, start, options)
	return goalResult("start goal", start.SessionID, result, err)
}

func (r *Connection) ResumeGoal(ctx context.Context, sessionID string) (protocol.Goal, error) {
	request, err := newGoalRequest("resume goal", sessionID)
	if err != nil {
		return protocol.Goal{}, err
	}
	result, err := r.goals.ResumeGoal(ctx, request, r.commandOptions())
	return goalResult("resume goal", sessionID, result, err)
}

func newGoalRequest(operation, sessionID string) (protocol.GoalRequest, error) {
	request := protocol.GoalRequest{SessionID: sessionID}
	if err := protocol.ValidateWireTree(request); err != nil {
		return protocol.GoalRequest{}, fmt.Errorf("%s: %w", operation, err)
	}
	return request, nil
}

func goalResult(operation, expectedSessionID string, result *protocol.Goal, err error) (protocol.Goal, error) {
	if err != nil {
		return protocol.Goal{}, classifyError(err)
	}
	if result == nil {
		return protocol.Goal{}, runtimeContractViolation("%s returned nil", operation)
	}
	if result.SessionID != expectedSessionID {
		return protocol.Goal{}, runtimeContractViolation(
			"%s returned session %q for %q",
			operation,
			result.SessionID,
			expectedSessionID,
		)
	}
	return *result, nil
}

func cloneGoal(value protocol.Goal) protocol.Goal {
	if value.Reason != nil {
		value.Reason = new(*value.Reason)
	}
	value.Used.CostUSD = clonePointer(value.Used.CostUSD)
	return value
}
