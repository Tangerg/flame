package delivery

import (
	"context"
	"errors"

	"github.com/Tangerg/flame/runtime/internal/application/agent/sessions"
	"github.com/Tangerg/flame/runtime/protocol"
)

// SessionUsage projects the application-owned session usage report onto the
// usage.session wire contract.
func (s *Handler) SessionUsage(ctx context.Context, sessionID string) (*protocol.SessionUsage, error) {
	report, err := s.usage.Session(ctx, sessionID)
	if err != nil {
		return nil, err
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
			return nil, NewFailure(errors.Join(protocol.ErrInvalidParams, periodErr), periodErr.Error())
		}
		period = recent
	}
	report, err := s.usage.Summary(ctx, period)
	if err != nil {
		return nil, err
	}
	return &protocol.UsageSummary{
		Total:      presentModelUsage(report.Total),
		ByProvider: presentUsageBuckets(report.ByProvider),
		ByModel:    presentUsageBuckets(report.ByModel),
		ByDay:      presentUsageBuckets(report.ByDay),
		Sessions:   report.Sessions,
		Runs:       report.Runs,
	}, nil
}

func presentSessionUsage(report sessions.UsageReport) *protocol.SessionUsage {
	return &protocol.SessionUsage{ModelUsage: presentModelUsage(report.Total), ByModel: presentUsageBuckets(report.ByModel)}
}

func presentUsageBuckets(buckets []sessions.UsageBucket) []protocol.UsageBucket {
	out := make([]protocol.UsageBucket, 0, len(buckets))
	for _, bucket := range buckets {
		out = append(out, protocol.UsageBucket{Key: bucket.Key, ModelUsage: presentModelUsage(bucket.Usage), Runs: bucket.Runs})
	}
	return out
}
