package runtimebinding

import (
	"context"
	"fmt"

	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

type usageBinding interface {
	GetSessionUsage(context.Context, protocol.SessionUsageRequest, protocol.CallOptions) (*protocol.SessionUsage, error)
	GetUsageSummary(context.Context, protocol.UsageSummaryRequest, protocol.CallOptions) (*protocol.UsageSummary, error)
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
	return conversation.SessionUsageReport{
		SessionID: request.SessionID,
		Total:     result.Total,
		Coverage:  result.Coverage, RecordedCostUSD: result.RecordedCostUSD,
		ByModel: result.ByModel,
	}, nil
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
		Period: period, Total: result.Total, Coverage: result.Coverage, RecordedCostUSD: result.RecordedCostUSD,
		ByProvider: result.ByProvider,
		ByModel:    result.ByModel,
		ByDay:      result.ByDay,
		ByOrigin:   result.ByOrigin, ByScope: result.ByScope,
		Sessions: result.Sessions, Runs: result.Runs,
	}
	return summary, nil
}
