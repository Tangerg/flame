package delivery

import (
	"context"
	"errors"

	"github.com/Tangerg/flame/runtime/internal/application/agent/sessions"
	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
	"github.com/Tangerg/flame/runtime/protocol"
)

// SessionUsage projects the application-owned session usage report onto the
// usage.session wire contract.
func (s *Handler) SessionUsage(ctx context.Context, sessionID string) (*protocol.SessionUsage, error) {
	report, err := s.usage.Session(ctx, sessionID)
	if err != nil {
		return nil, wireSessionErr(err)
	}
	return presentSessionUsage(report), nil
}

// UsageSummary projects the application-owned aggregate usage report onto the
// usage.summary wire contract.
func (s *Handler) UsageSummary(ctx context.Context, in protocol.UsageSummaryRequest) (*protocol.UsageSummary, error) {
	period := sessions.AllTimeUsage()
	if in.SinceDays != nil {
		recent, periodErr := sessions.RecentUsageDays(*in.SinceDays)
		if periodErr != nil {
			return nil, protocol.NewFailure(errors.Join(protocol.ErrInvalidParams, periodErr), periodErr.Error())
		}
		period = recent
	}
	report, err := s.usage.Summary(ctx, period)
	if err != nil {
		return nil, err
	}
	summary := &protocol.UsageSummary{
		RecordedUsage: presentRecordedUsage(report.Total, report.RecordedCostUSD, report.Coverage),
		ObservedAt:    report.ObservedAt,
		ByProvider:    presentUsageBuckets(report.ByProvider),
		ByModel:       presentUsageBuckets(report.ByModel),
		ByDay:         presentUsageBuckets(report.ByDay),
		ByOrigin:      presentUsageBuckets(report.ByOrigin),
		ByScope:       presentUsageBuckets(report.ByScope),
		Sessions:      report.Sessions,
		Runs:          report.Runs,
	}
	if !report.Since.IsZero() {
		summary.Since = new(report.Since)
	}
	return summary, nil
}

func presentSessionUsage(report sessions.UsageReport) *protocol.SessionUsage {
	return &protocol.SessionUsage{RecordedUsage: presentRecordedUsage(report.Total, report.RecordedCostUSD, report.Coverage), ByModel: presentUsageBuckets(report.ByModel)}
}

func presentUsageBuckets(buckets []sessions.UsageBucket) []protocol.UsageBucket {
	out := make([]protocol.UsageBucket, 0, len(buckets))
	for _, bucket := range buckets {
		out = append(out, protocol.UsageBucket{Key: bucket.Key, RecordedUsage: presentRecordedUsage(bucket.Usage, bucket.RecordedCostUSD, bucket.Coverage)})
	}
	return out
}

func presentRecordedUsage(total accounting.Totals, cost *float64, coverage sessions.UsageCoverage) protocol.RecordedUsage {
	return protocol.RecordedUsage{Total: presentModelUsage(total), RecordedCostUSD: cost, Coverage: protocol.UsageCoverage{Calls: coverage.Calls, ReportedCalls: coverage.ReportedCalls, PricedCalls: coverage.PricedCalls, StartedCalls: coverage.StartedCalls, CompletedCalls: coverage.CompletedCalls, FailedCalls: coverage.FailedCalls, UnknownCalls: coverage.UnknownCalls}}
}
