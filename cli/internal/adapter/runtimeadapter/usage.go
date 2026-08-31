package runtimeadapter

import (
	"context"
	"errors"
	"sort"

	flameruntime "github.com/Tangerg/flame/runtime"
	"github.com/Tangerg/flame/runtime/protocol"

	"github.com/Tangerg/flame/cli/internal/domain/agent"
)

type usageBinding interface {
	GetSessionUsage(context.Context, protocol.SessionUsageRequest, flameruntime.CallOptions) (*protocol.Usage, error)
	GetUsageSummary(context.Context, protocol.UsageSummaryRequest, flameruntime.CallOptions) (*protocol.UsageSummary, error)
}

var _ agent.UsageService = (*Connection)(nil)

func (r *Connection) SessionUsage(ctx context.Context, sessionID string) (agent.SessionUsageReport, error) {
	if sessionID == "" {
		return agent.SessionUsageReport{}, errors.New("session usage: session id is empty")
	}
	result, err := r.usage.GetSessionUsage(ctx, protocol.SessionUsageRequest{SessionID: sessionID}, r.callOptions())
	if err != nil {
		return agent.SessionUsageReport{}, classifyError(err)
	}
	if result == nil {
		return agent.SessionUsageReport{}, runtimeContractViolation("session usage returned nil")
	}
	report := agent.SessionUsageReport{
		SessionID: sessionID,
		Total:     projectUsageTotals(result.ModelUsage),
		ByModel:   make([]agent.UsageBucket, 0, len(result.ByModel)),
	}
	keys := make([]string, 0, len(result.ByModel))
	for key := range result.ByModel {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		report.ByModel = append(report.ByModel, agent.UsageBucket{Key: key, Totals: projectUsageTotals(result.ByModel[key])})
	}
	if err := report.Validate(); err != nil {
		return agent.SessionUsageReport{}, runtimeContractViolation("session usage returned an invalid report: %v", err)
	}
	return report, nil
}

func (r *Connection) Summary(ctx context.Context, period agent.UsageSummaryPeriod) (agent.UsageSummary, error) {
	days, recent, err := period.Days()
	if err != nil {
		return agent.UsageSummary{}, err
	}
	var sinceDays *int
	if recent {
		sinceDays = protocolPositiveInt(days)
	}
	result, err := r.usage.GetUsageSummary(ctx, protocol.UsageSummaryRequest{SinceDays: sinceDays}, r.callOptions())
	if err != nil {
		return agent.UsageSummary{}, classifyError(err)
	}
	if result == nil {
		return agent.UsageSummary{}, runtimeContractViolation("usage summary returned nil")
	}
	summary := agent.UsageSummary{
		Period: period, Total: projectUsageTotals(result.Total),
		ByProvider: projectUsageBuckets(result.ByProvider),
		ByModel:    projectUsageBuckets(result.ByModel),
		ByDay:      projectUsageBuckets(result.ByDay),
		Sessions:   result.Sessions, Runs: result.Runs,
	}
	if err := summary.Validate(); err != nil {
		return agent.UsageSummary{}, runtimeContractViolation("usage summary returned an invalid report: %v", err)
	}
	return summary, nil
}

func projectUsageBuckets(values []protocol.UsageBucket) []agent.UsageBucket {
	projected := make([]agent.UsageBucket, len(values))
	for index, value := range values {
		projected[index] = agent.UsageBucket{Key: value.Key, Totals: projectUsageTotals(value.ModelUsage), Runs: value.Runs}
	}
	return projected
}

func projectUsageTotals(value protocol.ModelUsage) agent.UsageTotals {
	projected := agent.UsageTotals{
		InputTokens: value.InputTokens, OutputTokens: value.OutputTokens,
		CacheReadTokens: value.CacheReadTokens, CacheWriteTokens: value.CacheWriteTokens,
		ReasoningTokens: value.ReasoningTokens,
	}
	if value.CostUSD != nil {
		projected.CostUSD = new(*value.CostUSD)
	}
	return projected
}
