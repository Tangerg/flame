package delivery

import (
	"context"

	"github.com/Tangerg/flame/runtime/protocol"
)

const (
	UsageSession Name = "usage.session"
	UsageSummary Name = "usage.summary"
)

func registerUsage(registry *Registry) {
	registry.Query(MethodMeta{
		Name: UsageSession, Errors: []string{protocol.ErrSessionNotFound.Error()},
	}, func(service *Handler, ctx context.Context, request protocol.SessionUsageRequest) (*protocol.Usage, error) {
		return service.SessionUsage(ctx, request.SessionID)
	})

	registry.Query(MethodMeta{Name: UsageSummary},
		(*Handler).UsageSummary)
}
