package runtimeadapter

import (
	"context"
	"errors"
	"sort"

	flameruntime "github.com/Tangerg/flame/runtime"
	"github.com/Tangerg/flame/runtime/protocol"

	"github.com/Tangerg/flame/cli/internal/usage"
)

type usageBinding interface {
	GetSessionUsage(context.Context, protocol.SessionUsageRequest, flameruntime.CallOptions) (*protocol.Usage, error)
	GetUsageSummary(context.Context, protocol.UsageSummaryRequest, flameruntime.CallOptions) (*protocol.UsageSummary, error)
}

var _ usage.Service = (*Connection)(nil)

func (r *Connection) SessionUsage(ctx context.Context, sessionID string) (usage.SessionReport, error) {
	if sessionID == "" {
		return usage.SessionReport{}, errors.New("session usage: session id is empty")
	}
	result, err := r.usage.GetSessionUsage(ctx, protocol.SessionUsageRequest{SessionID: sessionID}, r.callOptions())
	if err != nil {
		return usage.SessionReport{}, classifyError(err)
	}
	if result == nil {
		return usage.SessionReport{}, runtimeContractViolation("session usage returned nil")
	}
	report := usage.SessionReport{
		SessionID: sessionID,
		Total:     projectUsageTotals(result.ModelUsage),
		ByModel:   make([]usage.Bucket, 0, len(result.ByModel)),
	}
	keys := make([]string, 0, len(result.ByModel))
	for key := range result.ByModel {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		report.ByModel = append(report.ByModel, usage.Bucket{Key: key, Totals: projectUsageTotals(result.ByModel[key])})
	}
	if err := report.Validate(); err != nil {
		return usage.SessionReport{}, runtimeContractViolation("session usage returned an invalid report: %v", err)
	}
	return report, nil
}

func (r *Connection) Summary(ctx context.Context, period usage.SummaryPeriod) (usage.Summary, error) {
	days, recent, err := period.Days()
	if err != nil {
		return usage.Summary{}, err
	}
	var sinceDays *int
	if recent {
		sinceDays = protocolPositiveInt(days)
	}
	result, err := r.usage.GetUsageSummary(ctx, protocol.UsageSummaryRequest{SinceDays: sinceDays}, r.callOptions())
	if err != nil {
		return usage.Summary{}, classifyError(err)
	}
	if result == nil {
		return usage.Summary{}, runtimeContractViolation("usage summary returned nil")
	}
	summary := usage.Summary{
		Period: period, Total: projectUsageTotals(result.Total),
		ByProvider: projectUsageBuckets(result.ByProvider),
		ByModel:    projectUsageBuckets(result.ByModel),
		ByDay:      projectUsageBuckets(result.ByDay),
		Sessions:   result.Sessions, Runs: result.Runs,
	}
	if err := summary.Validate(); err != nil {
		return usage.Summary{}, runtimeContractViolation("usage summary returned an invalid report: %v", err)
	}
	return summary, nil
}

func projectUsageBuckets(values []protocol.UsageBucket) []usage.Bucket {
	projected := make([]usage.Bucket, len(values))
	for index, value := range values {
		projected[index] = usage.Bucket{Key: value.Key, Totals: projectUsageTotals(value.ModelUsage), Runs: value.Runs}
	}
	return projected
}

func projectUsageTotals(value protocol.ModelUsage) usage.Totals {
	projected := usage.Totals{
		InputTokens: value.InputTokens, OutputTokens: value.OutputTokens,
		CacheReadTokens: value.CacheReadTokens, CacheWriteTokens: value.CacheWriteTokens,
		ReasoningTokens: value.ReasoningTokens,
	}
	if value.CostUSD != nil {
		projected.CostUSD = new(*value.CostUSD)
	}
	return projected
}
