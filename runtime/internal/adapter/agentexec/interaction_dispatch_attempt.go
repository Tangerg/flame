package agentexec

import (
	"context"
	"errors"
	"fmt"
	"sync"

	agent "github.com/Tangerg/scope/agent"
)

type dispatchAttemptContextKey struct{}

// dispatchAttempt is scoped to exactly one EffectRequest. It lets invocation
// decorators distinguish a definite failure before an external call from a
// projection failure after any call in the Effect has crossed that boundary.
type dispatchAttempt struct {
	effectID agent.EffectID

	mu                      sync.Mutex
	externalBoundaryCrossed bool
	projectionErr           error
}

func newDispatchAttempt(effectID agent.EffectID) *dispatchAttempt {
	return &dispatchAttempt{effectID: effectID}
}

func withDispatchAttempt(ctx context.Context, attempt *dispatchAttempt) context.Context {
	return context.WithValue(ctx, dispatchAttemptContextKey{}, attempt)
}

func dispatchAttemptFrom(ctx context.Context, effectID agent.EffectID) (*dispatchAttempt, error) {
	if ctx == nil {
		return nil, errors.New("agentexec: missing dispatch context")
	}
	attempt, ok := ctx.Value(dispatchAttemptContextKey{}).(*dispatchAttempt)
	if !ok || attempt == nil || attempt.effectID != effectID {
		return nil, fmt.Errorf("agentexec: invocation attribution does not match dispatch Effect %s", effectID)
	}
	return attempt, nil
}

func (d *dispatchAttempt) beginExternalCall() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.projectionErr != nil {
		return d.projectionErr
	}
	d.externalBoundaryCrossed = true
	return nil
}

func (d *dispatchAttempt) recordProjectionFailure(err error) {
	if err == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.projectionErr == nil {
		d.projectionErr = err
		return
	}
	d.projectionErr = errors.Join(d.projectionErr, err)
}

func (d *dispatchAttempt) indeterminateFailure() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.externalBoundaryCrossed || d.projectionErr == nil {
		return nil
	}
	return d.projectionErr
}

func (d *dispatchAttempt) crossedExternalBoundary() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.externalBoundaryCrossed
}
