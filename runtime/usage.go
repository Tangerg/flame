package runtime

import (
	"context"

	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/protocol"
)

// GetSessionUsage returns usage accumulated by one Session.
func (r *binding) GetSessionUsage(ctx context.Context, request protocol.SessionUsageRequest, options CallOptions) (*protocol.SessionUsage, error) {
	return r.invoke[protocol.SessionUsageRequest, *protocol.SessionUsage](ctx, delivery.UsageSession, request, callOptions(options))
}

// GetUsageSummary returns aggregated Runtime usage.
func (r *binding) GetUsageSummary(ctx context.Context, request protocol.UsageSummaryRequest, options CallOptions) (*protocol.UsageSummary, error) {
	return r.invoke[protocol.UsageSummaryRequest, *protocol.UsageSummary](ctx, delivery.UsageSummary, request, callOptions(options))
}
