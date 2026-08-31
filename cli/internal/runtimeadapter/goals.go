package runtimeadapter

import (
	"context"
	"errors"
	"fmt"

	flameruntime "github.com/Tangerg/flame/runtime"
	"github.com/Tangerg/flame/runtime/protocol"

	"github.com/Tangerg/flame/cli/internal/agent"
)

type goalBinding interface {
	GetGoal(context.Context, protocol.GoalRequest, flameruntime.CallOptions) (*protocol.Goal, error)
	StartGoal(context.Context, protocol.StartGoalRequest, flameruntime.CommandOptions) (*protocol.Goal, error)
	UpdateGoal(context.Context, protocol.UpdateGoalRequest, flameruntime.CommandOptions) (*protocol.Goal, error)
	ClearGoal(context.Context, protocol.GoalRequest, flameruntime.CommandOptions) error
	StopGoal(context.Context, protocol.GoalRequest, flameruntime.CommandOptions) (*protocol.Goal, error)
	ResumeGoal(context.Context, protocol.GoalRequest, flameruntime.CommandOptions) (*protocol.Goal, error)
}

func (r *Connection) UpdateGoal(ctx context.Context, update agent.UpdateGoal) (agent.Goal, error) {
	if err := update.Validate(); err != nil {
		return agent.Goal{}, err
	}
	options, err := r.commandOptions()
	if err != nil {
		return agent.Goal{}, err
	}
	result, err := r.goals.UpdateGoal(ctx, protocol.UpdateGoalRequest{
		SessionID: update.SessionID,
		Objective: update.Objective,
	}, options)
	projected, err := projectGoalResult("update goal", update.SessionID, result, err)
	if err != nil {
		return agent.Goal{}, err
	}
	if err := update.ValidateResult(projected); err != nil {
		return agent.Goal{}, runtimeContractViolation("update goal returned an invalid acknowledgement: %v", err)
	}
	return projected, nil
}

func (r *Connection) ClearGoal(ctx context.Context, sessionID string) error {
	if sessionID == "" {
		return errors.New("clear goal: session id is empty")
	}
	options, err := r.commandOptions()
	if err != nil {
		return err
	}
	if err := r.goals.ClearGoal(ctx, protocol.GoalRequest{SessionID: sessionID}, options); err != nil {
		return classifyError(err)
	}
	return nil
}

var _ agent.GoalService = (*Connection)(nil)

func (r *Connection) GetGoal(ctx context.Context, sessionID string) (agent.Goal, bool, error) {
	if sessionID == "" {
		return agent.Goal{}, false, errors.New("get goal: session id is empty")
	}
	result, err := r.goals.GetGoal(ctx, protocol.GoalRequest{SessionID: sessionID}, r.callOptions())
	if err != nil {
		return agent.Goal{}, false, classifyError(err)
	}
	if result == nil {
		return agent.Goal{}, false, nil
	}
	projected, err := projectGoalResult("get goal", sessionID, result, nil)
	return projected, err == nil, err
}

func (r *Connection) StartGoal(ctx context.Context, start agent.StartGoal) (agent.Goal, error) {
	if err := start.Validate(); err != nil {
		return agent.Goal{}, err
	}
	options, err := r.commandOptions()
	if err != nil {
		return agent.Goal{}, err
	}
	result, err := r.goals.StartGoal(ctx, protocol.StartGoalRequest{
		SessionID: start.SessionID, Objective: start.Objective,
		Provider: start.Provider, Model: start.Model,
		Budget: goalBudgetToProtocol(start.Budget),
	}, options)
	projected, err := projectGoalResult("start goal", start.SessionID, result, err)
	if err != nil {
		return agent.Goal{}, err
	}
	if err := start.ValidateResult(projected); err != nil {
		return agent.Goal{}, runtimeContractViolation("start goal returned an invalid acknowledgement: %v", err)
	}
	return projected, nil
}

func (r *Connection) StopGoal(ctx context.Context, sessionID string) (agent.Goal, error) {
	projected, err := r.changeGoal(ctx, "stop goal", sessionID, r.goals.StopGoal)
	if err != nil {
		return agent.Goal{}, err
	}
	if projected.Status() == agent.GoalActive {
		return agent.Goal{}, runtimeContractViolation("stop goal returned an active acknowledgement")
	}
	return projected, nil
}

func (r *Connection) ResumeGoal(ctx context.Context, sessionID string) (agent.Goal, error) {
	projected, err := r.changeGoal(ctx, "resume goal", sessionID, r.goals.ResumeGoal)
	if err != nil {
		return agent.Goal{}, err
	}
	if projected.Status() != agent.GoalActive {
		return agent.Goal{}, runtimeContractViolation(
			"resume goal returned status %q, want %q",
			projected.Status(),
			agent.GoalActive,
		)
	}
	return projected, nil
}

func (r *Connection) changeGoal(
	ctx context.Context,
	operation, sessionID string,
	change func(context.Context, protocol.GoalRequest, flameruntime.CommandOptions) (*protocol.Goal, error),
) (agent.Goal, error) {
	if sessionID == "" {
		return agent.Goal{}, fmt.Errorf("%s: session id is empty", operation)
	}
	options, err := r.commandOptions()
	if err != nil {
		return agent.Goal{}, err
	}
	result, err := change(ctx, protocol.GoalRequest{SessionID: sessionID}, options)
	return projectGoalResult(operation, sessionID, result, err)
}

func projectGoalResult(operation, expectedSessionID string, result *protocol.Goal, err error) (agent.Goal, error) {
	if err != nil {
		return agent.Goal{}, classifyError(err)
	}
	if result == nil {
		return agent.Goal{}, runtimeContractViolation("%s returned nil", operation)
	}
	projected, err := projectGoal(*result)
	if err != nil {
		return agent.Goal{}, runtimeContractViolation("%s returned an invalid goal: %v", operation, err)
	}
	if projected.SessionID() != expectedSessionID {
		return agent.Goal{}, runtimeContractViolation(
			"%s returned session %q for %q",
			operation,
			projected.SessionID(),
			expectedSessionID,
		)
	}
	return projected, nil
}

func projectGoal(value protocol.Goal) (agent.Goal, error) {
	budget, err := projectGoalBudget(value.Budget)
	if err != nil {
		return agent.Goal{}, fmt.Errorf("project goal budget: %w", err)
	}
	used, err := agent.NewGoalUsage(value.Used.Runs, value.Used.CostUSD, value.Used.Steps)
	if err != nil {
		return agent.Goal{}, fmt.Errorf("project goal usage: %w", err)
	}
	snapshot := agent.GoalSnapshot{
		SessionID: value.SessionID, Objective: value.Objective, Status: agent.GoalStatus(value.Status),
		Provider: value.Provider, Model: value.Model, Budget: budget, Used: used,
		CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
	}
	if value.Reason != nil {
		snapshot.ReasonCode = agent.GoalReasonCode(value.Reason.Code)
		snapshot.ReasonDetail = value.Reason.Detail
	}
	projected, err := agent.RestoreGoal(snapshot)
	if err != nil {
		return agent.Goal{}, fmt.Errorf("project goal: %w", err)
	}
	return projected, nil
}

func goalBudgetToProtocol(budget agent.GoalBudget) *protocol.GoalBudget {
	if budget.Unlimited() {
		return nil
	}
	wire := &protocol.GoalBudget{}
	if value, limited := budget.MaxRuns(); limited {
		wire.MaxRuns = &value
	}
	if value, limited := budget.MaxCostUSD(); limited {
		wire.MaxCostUSD = &value
	}
	if value, limited := budget.MaxSteps(); limited {
		wire.MaxSteps = &value
	}
	return wire
}

func projectGoalBudget(wire *protocol.GoalBudget) (agent.GoalBudget, error) {
	if wire == nil {
		return agent.UnlimitedGoalBudget(), nil
	}
	return agent.NewGoalBudget(agent.GoalBudgetLimits{
		MaxRuns: wire.MaxRuns, MaxCostUSD: wire.MaxCostUSD, MaxSteps: wire.MaxSteps,
	})
}
