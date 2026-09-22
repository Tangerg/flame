package agentexec

import (
	"context"

	agent "github.com/Tangerg/scope/agent"
	"github.com/Tangerg/scope/agent/strategy/interaction"
)

// Scope owns definite settlements and unknown dispatch outcomes. Runtime gates
// dispatch on the active product Segment and retains local failure diagnostics.
type interactionDispatcher struct {
	inner   *interaction.Dispatcher
	session *interactionSession
}

func (i *interactionDispatcher) Dispatch(
	ctx context.Context,
	request agent.EffectRequest,
	emit agent.DeltaEmitter,
) (settlement agent.Settlement, err error) {
	defer func() {
		if err != nil {
			i.session.effectFailures.record(request.ID(), err)
			i.session.lifetime.wakeUnknown()
		}
	}()
	if err := i.session.awaitDispatchSegment(ctx); err != nil {
		return agent.Settlement{}, err
	}
	return i.inner.Dispatch(ctx, request, emit)
}

func (i *interactionDispatcher) ReplayPolicy(effect agent.Effect) agent.ReplayPolicy {
	return i.inner.ReplayPolicy(effect)
}
