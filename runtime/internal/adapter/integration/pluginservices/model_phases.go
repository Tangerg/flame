package pluginservices

import (
	"context"
	"fmt"
	"slices"

	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
)

type contributionPhaseKey struct{}

func enterContributionPhase(ctx context.Context, binding plugins.ContributionBinding) (context.Context, error) {
	active, _ := ctx.Value(contributionPhaseKey{}).([]plugins.ContributionBinding)
	if slices.ContainsFunc(active, binding.SameContribution) {
		return nil, fmt.Errorf("%w: recursive contribution %s", plugin.ErrInvalid, binding.ContributionID)
	}
	return context.WithValue(ctx, contributionPhaseKey{}, append(slices.Clone(active), binding)), nil
}

func modelContinuationContext(captured, request context.Context) (context.Context, func()) {
	ctx, cancel := context.WithCancelCause(captured)
	stop := context.AfterFunc(request, func() { cancel(context.Cause(request)) })
	if err := request.Err(); err != nil {
		cancel(context.Cause(request))
	}
	return ctx, func() {
		stop()
		cancel(nil)
	}
}
