package agent

import (
	"reflect"
	"slices"
	"time"

	"github.com/Tangerg/flame/cli/internal/domain/failure"
	"github.com/Tangerg/flame/runtime/protocol"
)

// CloneRun acquires the mutable facts retained by Conversation or a view.
func CloneRun(value protocol.RunRef) protocol.RunRef {
	value.Outcome = cloneRunOutcome(value.Outcome)
	value.Metrics = CloneRunMetrics(value.Metrics)
	if value.Limits != nil {
		limits := *value.Limits
		if limits.MaxTotalTokens != nil {
			limits.MaxTotalTokens = new(*limits.MaxTotalTokens)
		}
		if limits.MaxSteps != nil {
			limits.MaxSteps = new(*limits.MaxSteps)
		}
		if limits.MaxBudgetUSD != nil {
			limits.MaxBudgetUSD = new(*limits.MaxBudgetUSD)
		}
		value.Limits = &limits
	}
	value.ProtocolProfile.RequiredFeatures = slices.Clone(value.ProtocolProfile.RequiredFeatures)
	value.ProtocolProfile.InterruptTypes = slices.Clone(value.ProtocolProfile.InterruptTypes)
	return value
}

// equalRuns compares observed facts when reconciling replay and cold reads.
func equalRuns(left, right protocol.RunRef) bool {
	left.CreatedAt, right.CreatedAt = left.CreatedAt.UTC(), right.CreatedAt.UTC()
	left.FinishedAt, right.FinishedAt = left.FinishedAt.UTC(), right.FinishedAt.UTC()
	return reflect.DeepEqual(left, right)
}

// UsageFromMetrics translates Runtime metering into the terminal's display units.
func UsageFromMetrics(metrics protocol.RunMetrics) Usage {
	value := Usage{Steps: metrics.Steps, Duration: time.Duration(metrics.ActiveDurationMillis) * time.Millisecond}
	if metrics.Usage != nil {
		u := metrics.Usage
		value.InputTokens = u.InputTokens
		value.OutputTokens = u.OutputTokens
		value.CacheReadTokens = u.CacheReadTokens
		value.CacheWriteTokens = u.CacheWriteTokens
		value.ReasoningTokens = u.ReasoningTokens
		value.CostUSD = u.CostUSD
		value.ByModel = u.ByModel
	}
	return value.Clone()
}

func OutcomeFromRun(value *protocol.RunOutcome) Outcome {
	if value == nil {
		return Outcome{}
	}
	outcome := Outcome{Status: protocol.RunOutcomeType(value.Type), Detail: value.Detail}
	switch value.Type {
	case protocol.OutcomeTimedOut, protocol.OutcomeFailed, protocol.OutcomeLost:
		outcome.Detail = ""
		outcome.Problem = failure.Clone(value.Error)
	}
	return outcome
}

func (o Outcome) RunOutcome() *protocol.RunOutcome {
	if o.Status == "" {
		return nil
	}
	return &protocol.RunOutcome{Type: protocol.RunOutcomeType(o.Status), Detail: o.Detail, Error: failure.Clone(o.Problem)}
}

func cloneRunOutcome(value *protocol.RunOutcome) *protocol.RunOutcome {
	if value == nil {
		return nil
	}
	cloned := *value
	cloned.Error = failure.Clone(value.Error)
	return &cloned
}

// CloneRunMetrics acquires mutable metering observations retained across events.
func CloneRunMetrics(value protocol.RunMetrics) protocol.RunMetrics {
	if value.Usage != nil {
		usage := *value.Usage
		if usage.CostUSD != nil {
			usage.CostUSD = new(*usage.CostUSD)
		}
		if usage.ByModel != nil {
			byModel := make(map[string]protocol.ModelUsage, len(usage.ByModel))
			for id, model := range usage.ByModel {
				byModel[id] = cloneProtocolModelUsage(model)
			}
			usage.ByModel = byModel
		}
		value.Usage = &usage
	}
	return value
}
