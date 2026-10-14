package pluginservices

import (
	"context"
	"fmt"
	"slices"

	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
)

type modelPhaseKey struct{}

// A fresh nested exchange cannot reenter a contributor still handling its
// ancestor. Keep the active path immutable across concurrent callback requests;
// rejecting a cycle does not skip a contributor or change the selected chain.
func enterModelPhase(ctx context.Context, binding plugins.ContributionBinding) (context.Context, error) {
	active, _ := ctx.Value(modelPhaseKey{}).([]plugins.ContributionBinding)
	if slices.Contains(active, binding) {
		return nil, fmt.Errorf("%w: recursive model contributor %s", plugin.ErrInvalid, binding.ContributionID)
	}
	return context.WithValue(ctx, modelPhaseKey{}, append(slices.Clone(active), binding)), nil
}

// RPC belongs to the receiving activation, but next keeps the original caller's
// values and frozen chain. Its network request may cancel that original work.
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
