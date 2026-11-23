package mcp

import (
	"context"

	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"

	framehost "github.com/Tangerg/frame/host"
)

// ValidateCapturedBindingUnder accepts only a live catalog capture or a graph
// retained by an original command/checkpoint owner. A serialized graph alone
// cannot authorize a newly published execution.
func (p *Pool) ValidateCapturedBindingUnder(ctx context.Context, activation plugin.Activation) error {
	host := p.installed.host
	if err := host.Admission().CheckAdmitted(ctx); err != nil {
		return err
	}
	if _, err := plugins.DecodeActivation(activation); err != nil {
		return err
	}
	if captured := p.capturedObservation(activation); captured != nil && captured.plan.Valid() {
		return validateObservedPlan(ctx, host, captured, activation)
	}
	permission := p.installed.permission
	permission.mu.Lock()
	grant := permission.scope
	permission.mu.Unlock()
	binding, err := permission.Read(ctx, activation.Identity().String(), grant)
	if err != nil {
		return err
	}
	return host.ValidateBindingUnder(ctx, binding)
}

// capturedObservation is the live installed session still serving activation.
func (p *Pool) capturedObservation(activation plugin.Activation) *installedObservation {
	p.inner.mu.Lock()
	defer p.inner.mu.Unlock()
	if p.inner.closed {
		return nil
	}
	for _, candidate := range p.inner.sessions {
		if candidate.managed == nil || candidate.close != nil {
			continue
		}
		binding, err := candidate.managed.activation()
		if err == nil && binding.Equal(activation) {
			return candidate.managed
		}
	}
	return nil
}

// validateObservedPlan proves the captured session still runs the same plan on
// a ready instance.
func validateObservedPlan(ctx context.Context, host framehost.Host, captured *installedObservation, activation plugin.Activation) error {
	binding, err := host.ValidatePlanUnder(ctx, captured.plan)
	if err != nil {
		return err
	}
	original, err := plugins.ProjectActivation(binding)
	if err != nil {
		return err
	}
	if !original.Equal(activation) {
		return framehost.ErrStaleBinding
	}
	for _, instance := range host.Observations() {
		if instance.ID == captured.instanceID && instance.State == framehost.Ready {
			return nil
		}
	}
	return framehost.ErrStaleBinding
}
