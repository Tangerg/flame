package runtimebinding

import (
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/Tangerg/flame/runtime/protocol"

	"github.com/Tangerg/flame/cli/internal/domain/agent"
	"github.com/Tangerg/flame/cli/internal/domain/failure"
)

func projectRun(value protocol.RunRef) (agent.Run, error) {
	if err := protocol.ValidateWireTree(value); err != nil {
		return agent.Run{}, fmt.Errorf("run %s wire projection: %w", value.ID, err)
	}
	lineage, err := projectRunLineage(value)
	if err != nil {
		return agent.Run{}, fmt.Errorf("run %s: %w", value.ID, err)
	}
	projected := agent.Run{
		ID: value.ID, SessionID: value.SessionID,
		Provider: value.Provider, Model: value.Model, ReasoningEffort: value.ReasoningEffort,
		Lineage: lineage,
		Status:  value.Status, ActiveSegmentID: value.ActiveSegmentID,
		CreatedAt: value.CreatedAt, FinishedAt: value.FinishedAt,
		Limits: agent.UnlimitedRunLimits(), ContextTokens: value.ContextTokens,
		Usage: projectUsage(value.Metrics), ProtocolProfile: projectRunProtocolProfile(value.ProtocolProfile),
	}
	if value.Limits != nil {
		projected.Limits, err = agent.NewRunLimits(agent.RunLimitValues{
			MaxTotalTokens: value.Limits.MaxTotalTokens,
			MaxSteps:       value.Limits.MaxSteps,
			MaxBudgetUSD:   value.Limits.MaxBudgetUSD,
		})
		if err != nil {
			return agent.Run{}, fmt.Errorf("runtime run %s limits: %w", value.ID, err)
		}
	}
	if value.Outcome != nil {
		outcome, err := projectRunOutcome(*value.Outcome)
		if err != nil {
			return agent.Run{}, fmt.Errorf("runtime run %s outcome: %w", value.ID, err)
		}
		projected.Outcome = outcome
	}
	if err := projected.Validate(); err != nil {
		return agent.Run{}, fmt.Errorf("runtime run %s: %w", value.ID, err)
	}
	return projected, nil
}

func projectRunLineage(value protocol.RunRef) (agent.RunLineage, error) {
	if value.SpawnedByItemID == "" && value.ParentRunID == "" && value.RootRunID == "" {
		return agent.RootRunLineage(), nil
	}
	return agent.NewChildRunLineage(value.ID, value.SpawnedByItemID, value.ParentRunID, value.RootRunID)
}

func projectRunProtocolProfile(profile protocol.RunProtocolProfile) *protocol.RunProtocolProfile {
	projected := profile
	projected.RequiredFeatures = slices.Clone(profile.RequiredFeatures)
	projected.InterruptTypes = slices.Clone(profile.InterruptTypes)
	return &projected
}

func projectUsage(metrics protocol.RunMetrics) agent.Usage {
	usage := agent.Usage{
		Steps: metrics.Steps, Duration: time.Duration(metrics.ActiveDurationMillis) * time.Millisecond,
	}
	if metrics.Usage == nil {
		return usage
	}
	projected := projectUsageBreakdown(*metrics.Usage)
	projected.Steps, projected.Duration = usage.Steps, usage.Duration
	return projected
}

func projectUsageBreakdown(value protocol.Usage) agent.Usage {
	usage := agent.Usage{
		InputTokens: value.InputTokens, OutputTokens: value.OutputTokens,
		CacheReadTokens: value.CacheReadTokens, CacheWriteTokens: value.CacheWriteTokens,
		ReasoningTokens: value.ReasoningTokens, ByModel: cloneUsageByModel(value.ByModel),
	}
	if value.CostUSD != nil {
		usage.CostUSD = new(*value.CostUSD)
	}
	return usage
}

func cloneUsageByModel(values map[string]protocol.ModelUsage) map[string]protocol.ModelUsage {
	if values == nil {
		return nil
	}
	projected := make(map[string]protocol.ModelUsage, len(values))
	for model, value := range values {
		projected[model] = cloneModelUsage(value)
	}
	return projected
}

func projectRunOutcome(value protocol.RunOutcome) (agent.Outcome, error) {
	return projectOutcome(protocol.SegmentOutcome{
		Type: protocol.SegmentOutcomeType(value.Type), Error: value.Error, Detail: value.Detail,
	})
}

func projectOutcome(value protocol.SegmentOutcome) (agent.Outcome, error) {
	if err := protocol.ValidateWireTree(value); err != nil {
		return agent.Outcome{}, runtimeContractViolation("runtime outcome is invalid: %v", err)
	}
	outcome := agent.Outcome{Status: agent.OutcomeStatus(value.Type), Detail: value.Detail}
	switch value.Type {
	case protocol.SegmentTimedOut, protocol.SegmentFailed, protocol.SegmentLost:
		outcome.Detail = ""
		outcome.Problem = failure.Clone(value.Error)
	}
	return outcome, nil
}

func projectPlan(plan *protocol.Plan) (*protocol.Plan, error) {
	if plan == nil {
		return nil, errors.New("plan projection is nil")
	}
	if err := protocol.ValidateWireTree(*plan); err != nil {
		return nil, err
	}
	if plan.State == nil {
		return nil, nil
	}
	projected := *plan
	state := *plan.State
	state.Steps = slices.Clone(plan.State.Steps)
	projected.State = &state
	return &projected, nil
}

func projectInteraction(value protocol.Interrupt) (agent.Interaction, error) {
	if err := protocol.ValidateWireTree(value); err != nil {
		return nil, fmt.Errorf("interrupt %s wire projection: %w", value.ItemID, err)
	}
	if value.Payload == nil {
		return nil, fmt.Errorf("interrupt %s has no payload", value.ItemID)
	}
	switch value.Type {
	case protocol.InterruptApproval:
		tool, err := projectTool(toolProjection{invocation: value.Payload.Tool, status: protocol.ItemStatusRunning})
		if err != nil {
			return nil, fmt.Errorf("approval %s: %w", value.ItemID, err)
		}
		approval := agent.Approval{
			RunID: value.RunID, ItemID: value.ItemID, Title: "Approve " + tool.Name, Detail: value.Payload.Reason,
			Tool: &tool, Risk: value.Payload.Risk, Rememberable: value.Payload.Rememberable,
		}
		if err := approval.Validate(); err != nil {
			return nil, err
		}
		return approval, nil
	case protocol.InterruptQuestion:
		question, err := projectQuestion(value.RunID, value.ItemID, value.Payload.Question)
		if err != nil {
			return nil, err
		}
		if err := agent.ValidateInteraction(question); err != nil {
			return nil, fmt.Errorf("question interrupt %s: %w", value.ItemID, err)
		}
		return question, nil
	default:
		return nil, fmt.Errorf("%w: interrupt type %q is unsupported", agent.ErrIncompatibleRuntime, value.Type)
	}
}

func projectInteractions(values []protocol.Interrupt) ([]agent.Interaction, error) {
	interactions := make([]agent.Interaction, 0, len(values))
	for _, value := range values {
		projected, err := projectInteraction(value)
		if err != nil {
			return nil, err
		}
		interactions = append(interactions, projected)
	}
	if err := agent.ValidateInteractions(interactions); err != nil {
		return nil, err
	}
	return interactions, nil
}
