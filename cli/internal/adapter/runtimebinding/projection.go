package runtimebinding

import (
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/Tangerg/flame/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/cli/internal/domain/failure"
	"github.com/Tangerg/flame/runtime/protocol"
)

func projectRun(value protocol.RunRef) (conversation.Run, error) {
	lineage, err := projectRunLineage(value)
	if err != nil {
		return conversation.Run{}, fmt.Errorf("run %s: %w", value.ID, err)
	}
	projected := conversation.Run{
		ID: value.ID, SessionID: value.SessionID,
		Provider: value.Provider, Model: value.Model, ReasoningEffort: value.ReasoningEffort,
		Lineage: lineage,
		Status:  value.Status, ActiveSegmentID: value.ActiveSegmentID,
		CreatedAt: value.CreatedAt, FinishedAt: value.FinishedAt,
		ContextTokens: value.ContextTokens,
		Usage:         projectUsage(value.Metrics), ProtocolProfile: projectRunProtocolProfile(value.ProtocolProfile),
	}
	if value.Outcome != nil {
		projected.Outcome = projectRunOutcome(*value.Outcome)
	}
	return projected, nil
}

func projectRunLineage(value protocol.RunRef) (conversation.RunLineage, error) {
	if value.SpawnedByItemID == "" && value.ParentRunID == "" && value.RootRunID == "" {
		return conversation.RootRunLineage(), nil
	}
	return conversation.NewChildRunLineage(value.ID, value.SpawnedByItemID, value.ParentRunID, value.RootRunID)
}

func projectRunProtocolProfile(profile *protocol.RunProtocolProfile) *protocol.RunProtocolProfile {
	if profile == nil {
		return nil
	}
	projected := *profile
	projected.RequiredFeatures = slices.Clone(profile.RequiredFeatures)
	projected.InterruptTypes = slices.Clone(profile.InterruptTypes)
	return &projected
}

func projectUsage(metrics protocol.RunMetrics) conversation.Usage {
	usage := conversation.Usage{
		Steps: metrics.Steps, Duration: time.Duration(metrics.ActiveDurationMillis) * time.Millisecond,
	}
	if metrics.Usage == nil {
		return usage
	}
	projected := projectUsageBreakdown(*metrics.Usage)
	projected.Steps, projected.Duration = usage.Steps, usage.Duration
	return projected
}

func projectUsageBreakdown(value protocol.Usage) conversation.Usage {
	usage := conversation.Usage{
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

// projectRunOutcome folds a terminal Run outcome into the CLI's flat
// presentation value. The wire contract already keeps Error and Detail on
// disjoint terminals, so each tag carries at most one of them.
func projectRunOutcome(value protocol.RunOutcome) conversation.Outcome {
	return conversation.Outcome{Status: value.Type, Detail: value.Detail, Problem: failure.Clone(value.Error)}
}

func projectPlan(plan *protocol.Plan) (*protocol.Plan, error) {
	if plan == nil {
		return nil, errors.New("plan projection is nil")
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

func projectInterrupt(value protocol.Interrupt) (conversation.Interrupt, error) {
	if value.Payload == nil {
		return nil, fmt.Errorf("interrupt %s has no payload", value.ItemID)
	}
	switch value.Type {
	case protocol.InterruptApproval:
		tool, err := projectTool(toolProjection{invocation: value.Payload.Tool, status: protocol.ItemStatusRunning})
		if err != nil {
			return nil, fmt.Errorf("approval %s: %w", value.ItemID, err)
		}
		return conversation.Approval{
			RunID: value.RunID, ItemID: value.ItemID, Title: "Approve " + tool.Name, Detail: value.Payload.Reason,
			Tool: &tool, Risk: value.Payload.Risk, Rememberable: value.Payload.Rememberable,
		}, nil
	case protocol.InterruptQuestion:
		return projectQuestion(value.RunID, value.ItemID, value.Payload.Question)
	default:
		return nil, fmt.Errorf("%w: interrupt type %q is unsupported", conversation.ErrIncompatibleRuntime, value.Type)
	}
}

func projectInterrupts(values []protocol.Interrupt) ([]conversation.Interrupt, error) {
	interrupts := make([]conversation.Interrupt, 0, len(values))
	for _, value := range values {
		projected, err := projectInterrupt(value)
		if err != nil {
			return nil, err
		}
		interrupts = append(interrupts, projected)
	}
	return interrupts, nil
}

func cloneModelUsage(value protocol.ModelUsage) protocol.ModelUsage {
	if value.CostUSD != nil {
		value.CostUSD = new(*value.CostUSD)
	}
	return value
}
