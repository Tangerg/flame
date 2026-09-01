package agentexec

import (
	"context"
	"fmt"
	"time"

	agent "github.com/Tangerg/scope/agent"
	"github.com/Tangerg/scope/agent/interaction"
)

const authoritativeProjectionTimeout = 15 * time.Second

// interactionDispatcher gives each EffectRequest one independent attempt
// tracker. The inner Interaction Dispatcher still owns protocol decoding and
// definite settlements; this wrapper alone converts a post-external-call
// projection failure into Agent Framework's unknown settlement path.
type interactionDispatcher struct {
	inner   *interaction.Dispatcher
	session *interactionSession
}

func (i *interactionDispatcher) Dispatch(
	ctx context.Context,
	request agent.EffectRequest,
	emit agent.DeltaEmitter,
) (agent.Settlement, error) {
	ctx, finishDispatch := i.session.beginDispatch(ctx, request)
	defer finishDispatch()
	attempt := newDispatchAttempt(ctx, request.ID())
	defer attempt.close()
	settlement, err := i.inner.Dispatch(withDispatchAttempt(ctx, attempt), request, emit)
	if projectionErr := attempt.indeterminateFailure(); projectionErr != nil {
		i.session.lifetime.wakeUnknown()
		return agent.Settlement{}, fmt.Errorf(
			"agentexec: authoritative projection failed after external Effect %s: %w",
			request.ID(), projectionErr,
		)
	}
	if err != nil && attempt.crossedExternalBoundary() {
		// The inner Dispatcher already returns an indeterminate error to Engine.
		// Wake the direct path as well; the periodic public-state reconciliation
		// remains the loss-tolerant backstop.
		i.session.lifetime.wakeUnknown()
	}
	return settlement, err
}

func (i *interactionDispatcher) ReplayPolicy(effect agent.Effect) agent.ReplayPolicy {
	return i.inner.ReplayPolicy(effect)
}
