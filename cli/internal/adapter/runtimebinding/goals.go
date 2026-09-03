package runtimebinding

import (
	"context"
	"fmt"

	flameruntime "github.com/Tangerg/flame/runtime"
	"github.com/Tangerg/flame/runtime/protocol"
)

type goalBinding interface {
	GetGoal(context.Context, protocol.GoalRequest, flameruntime.CallOptions) (*protocol.Goal, error)
	StartGoal(context.Context, protocol.StartGoalRequest, flameruntime.CommandOptions) (*protocol.Goal, error)
	UpdateGoal(context.Context, protocol.UpdateGoalRequest, flameruntime.CommandOptions) (*protocol.Goal, error)
	ClearGoal(context.Context, protocol.GoalRequest, flameruntime.CommandOptions) error
	StopGoal(context.Context, protocol.GoalRequest, flameruntime.CommandOptions) (*protocol.Goal, error)
	ResumeGoal(context.Context, protocol.GoalRequest, flameruntime.CommandOptions) (*protocol.Goal, error)
}

func (r *Connection) UpdateGoal(ctx context.Context, update protocol.UpdateGoalRequest) (protocol.Goal, error) {
	if err := protocol.ValidateWireTree(update); err != nil {
		return protocol.Goal{}, fmt.Errorf("update goal: %w", err)
	}
	options, err := r.commandOptions()
	if err != nil {
		return protocol.Goal{}, err
	}
	result, err := r.goals.UpdateGoal(ctx, update, options)
	updated, err := goalResult("update goal", update.SessionID, result, err)
	if err != nil {
		return protocol.Goal{}, err
	}
	if updated.Objective != update.Objective {
		return protocol.Goal{}, runtimeContractViolation(
			"update goal returned objective %q, want %q",
			updated.Objective,
			update.Objective,
		)
	}
	return updated, nil
}

func (r *Connection) ClearGoal(ctx context.Context, sessionID string) error {
	request, err := newGoalRequest("clear goal", sessionID)
	if err != nil {
		return err
	}
	options, err := r.commandOptions()
	if err != nil {
		return err
	}
	if err := r.goals.ClearGoal(ctx, request, options); err != nil {
		return classifyError(err)
	}
	return nil
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
	options, err := r.commandOptions()
	if err != nil {
		return protocol.Goal{}, err
	}
	result, err := r.goals.StartGoal(ctx, start, options)
	started, err := goalResult("start goal", start.SessionID, result, err)
	if err != nil {
		return protocol.Goal{}, err
	}
	if started.Objective != start.Objective || started.Status != protocol.GoalActive ||
		!goalStartSelectionAcknowledged(start, started) ||
		!equalGoalBudget(started.Budget, start.Budget) ||
		started.Used.Runs != 0 || started.Used.Steps != 0 || started.Used.CostUSD != nil {
		return protocol.Goal{}, runtimeContractViolation("start goal returned an acknowledgement that differs from the request")
	}
	return started, nil
}

func goalStartSelectionAcknowledged(start protocol.StartGoalRequest, started protocol.Goal) bool {
	if start.Provider == "" && start.Model == "" {
		// Omission delegates selection to the Session. Goal is the authoritative
		// resolved result, so there is no request value to echo here.
		return true
	}
	return started.Provider == start.Provider && started.Model == start.Model &&
		started.ReasoningEffort == start.ReasoningEffort
}

func (r *Connection) StopGoal(ctx context.Context, sessionID string) (protocol.Goal, error) {
	stopped, err := r.changeGoal(ctx, "stop goal", sessionID, r.goals.StopGoal)
	if err != nil {
		return protocol.Goal{}, err
	}
	if stopped.Status == protocol.GoalActive {
		return protocol.Goal{}, runtimeContractViolation("stop goal returned an active acknowledgement")
	}
	return stopped, nil
}

func (r *Connection) ResumeGoal(ctx context.Context, sessionID string) (protocol.Goal, error) {
	resumed, err := r.changeGoal(ctx, "resume goal", sessionID, r.goals.ResumeGoal)
	if err != nil {
		return protocol.Goal{}, err
	}
	if resumed.Status != protocol.GoalActive {
		return protocol.Goal{}, runtimeContractViolation(
			"resume goal returned status %q, want %q",
			resumed.Status,
			protocol.GoalActive,
		)
	}
	return resumed, nil
}

func (r *Connection) changeGoal(
	ctx context.Context,
	operation, sessionID string,
	change func(context.Context, protocol.GoalRequest, flameruntime.CommandOptions) (*protocol.Goal, error),
) (protocol.Goal, error) {
	request, err := newGoalRequest(operation, sessionID)
	if err != nil {
		return protocol.Goal{}, err
	}
	options, err := r.commandOptions()
	if err != nil {
		return protocol.Goal{}, err
	}
	result, err := change(ctx, request, options)
	return goalResult(operation, sessionID, result, err)
}

func newGoalRequest(operation, sessionID string) (protocol.GoalRequest, error) {
	request := protocol.GoalRequest{SessionID: sessionID}
	if err := request.ValidateWire(); err != nil {
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
	if err := protocol.ValidateWireTree(*result); err != nil {
		return protocol.Goal{}, runtimeContractViolation("%s returned an invalid goal: %v", operation, err)
	}
	if result.SessionID != expectedSessionID {
		return protocol.Goal{}, runtimeContractViolation(
			"%s returned session %q for %q",
			operation,
			result.SessionID,
			expectedSessionID,
		)
	}
	return cloneGoal(*result), nil
}

func cloneGoal(value protocol.Goal) protocol.Goal {
	if value.Reason != nil {
		value.Reason = new(*value.Reason)
	}
	if value.Budget != nil {
		budget := *value.Budget
		budget.MaxRuns = cloneOptional(budget.MaxRuns)
		budget.MaxCostUSD = cloneOptional(budget.MaxCostUSD)
		budget.MaxSteps = cloneOptional(budget.MaxSteps)
		value.Budget = &budget
	}
	value.Used.CostUSD = cloneOptional(value.Used.CostUSD)
	return value
}

func equalGoalBudget(left, right *protocol.GoalBudget) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return equalOptional(left.MaxRuns, right.MaxRuns) &&
		equalOptional(left.MaxCostUSD, right.MaxCostUSD) &&
		equalOptional(left.MaxSteps, right.MaxSteps)
}

func cloneOptional[T any](value *T) *T {
	if value == nil {
		return nil
	}
	return new(*value)
}

func equalOptional[T comparable](left, right *T) bool {
	return (left == nil) == (right == nil) && (left == nil || *left == *right)
}
