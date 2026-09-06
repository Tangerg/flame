package runtimebinding

import (
	"context"
	"fmt"
	"sort"

	flameruntime "github.com/Tangerg/flame/runtime"
	"github.com/Tangerg/flame/runtime/protocol"

	"github.com/Tangerg/flame/cli/internal/domain/agent"
)

type usageBinding interface {
	GetSessionUsage(context.Context, protocol.SessionUsageRequest, flameruntime.CallOptions) (*protocol.Usage, error)
	GetUsageSummary(context.Context, protocol.UsageSummaryRequest, flameruntime.CallOptions) (*protocol.UsageSummary, error)
}

func (r *Connection) SessionUsage(ctx context.Context, sessionID string) (agent.SessionUsageReport, error) {
	request := protocol.SessionUsageRequest{SessionID: sessionID}
	if err := request.ValidateWire(); err != nil {
		return agent.SessionUsageReport{}, fmt.Errorf("session usage: %w", err)
	}
	result, err := r.usage.GetSessionUsage(ctx, request, r.callOptions())
	if err != nil {
		return agent.SessionUsageReport{}, classifyError(err)
	}
	if result == nil {
		return agent.SessionUsageReport{}, runtimeContractViolation("session usage returned nil")
	}
	if err := protocol.ValidateWireTree(*result); err != nil {
		return agent.SessionUsageReport{}, runtimeContractViolation("session usage returned an invalid wire result: %v", err)
	}
	report := agent.SessionUsageReport{
		SessionID: request.SessionID,
		Total:     result.ModelUsage,
		ByModel:   make([]protocol.UsageBucket, 0, len(result.ByModel)),
	}
	keys := make([]string, 0, len(result.ByModel))
	for key := range result.ByModel {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		report.ByModel = append(report.ByModel, protocol.UsageBucket{Key: key, ModelUsage: result.ByModel[key]})
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
	request := protocol.UsageSummaryRequest{SinceDays: sinceDays}
	if err := request.ValidateWire(); err != nil {
		return agent.UsageSummary{}, fmt.Errorf("usage summary: %w", err)
	}
	result, err := r.usage.GetUsageSummary(ctx, request, r.callOptions())
	if err != nil {
		return agent.UsageSummary{}, classifyError(err)
	}
	if result == nil {
		return agent.UsageSummary{}, runtimeContractViolation("usage summary returned nil")
	}
	if err := protocol.ValidateWireTree(*result); err != nil {
		return agent.UsageSummary{}, runtimeContractViolation("usage summary returned an invalid wire result: %v", err)
	}
	summary := agent.UsageSummary{
		Period: period, Total: result.Total,
		ByProvider: result.ByProvider,
		ByModel:    result.ByModel,
		ByDay:      result.ByDay,
		Sessions:   result.Sessions, Runs: result.Runs,
	}
	if err := summary.Validate(); err != nil {
		return agent.UsageSummary{}, runtimeContractViolation("usage summary returned an invalid report: %v", err)
	}
	return summary, nil
}
