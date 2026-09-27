package runtimebinding

import (
	"context"
	"fmt"
	"sort"

	"github.com/Tangerg/flame/cli/internal/domain/conversation"
	flameruntime "github.com/Tangerg/flame/runtime"
	"github.com/Tangerg/flame/runtime/protocol"
)

type usageBinding interface {
	GetSessionUsage(context.Context, protocol.SessionUsageRequest, flameruntime.CallOptions) (*protocol.Usage, error)
	GetUsageSummary(context.Context, protocol.UsageSummaryRequest, flameruntime.CallOptions) (*protocol.UsageSummary, error)
}

func (r *Connection) SessionUsage(ctx context.Context, sessionID string) (conversation.SessionUsageReport, error) {
	request := protocol.SessionUsageRequest{SessionID: sessionID}
	if err := request.ValidateWire(); err != nil {
		return conversation.SessionUsageReport{}, fmt.Errorf("session usage: %w", err)
	}
	result, err := r.usage.GetSessionUsage(ctx, request, r.callOptions())
	if err != nil {
		return conversation.SessionUsageReport{}, classifyError(err)
	}
	if result == nil {
		return conversation.SessionUsageReport{}, runtimeContractViolation("session usage returned nil")
	}
	report := conversation.SessionUsageReport{
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
	return report, nil
}

func (r *Connection) Summary(ctx context.Context, period conversation.UsageSummaryPeriod) (conversation.UsageSummary, error) {
	days, recent, err := period.Days()
	if err != nil {
		return conversation.UsageSummary{}, err
	}
	var sinceDays *int
	if recent {
		sinceDays = protocolPositiveInt(days)
	}
	request := protocol.UsageSummaryRequest{SinceDays: sinceDays}
	if err := request.ValidateWire(); err != nil {
		return conversation.UsageSummary{}, fmt.Errorf("usage summary: %w", err)
	}
	result, err := r.usage.GetUsageSummary(ctx, request, r.callOptions())
	if err != nil {
		return conversation.UsageSummary{}, classifyError(err)
	}
	if result == nil {
		return conversation.UsageSummary{}, runtimeContractViolation("usage summary returned nil")
	}
	summary := conversation.UsageSummary{
		Period: period, Total: result.Total,
		ByProvider: result.ByProvider,
		ByModel:    result.ByModel,
		ByDay:      result.ByDay,
		Sessions:   result.Sessions, Runs: result.Runs,
	}
	return summary, nil
}
