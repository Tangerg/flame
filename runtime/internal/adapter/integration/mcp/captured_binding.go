package mcp

import (
	"context"

	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/frame"
)

// ValidateCapturedBindingUnder accepts only a live catalog capture or a graph
// retained by an original command/checkpoint owner. A serialized graph alone
// cannot authorize a newly published execution.
func (p *Pool) ValidateCapturedBindingUnder(ctx context.Context, activation plugin.Activation) error {
	if p.installed == nil {
		return ErrUnavailable
	}
	host := p.installed.host
	if err := host.Admission().CheckAdmitted(ctx); err != nil {
		return err
	}
	if _, err := plugins.DecodeActivation(activation); err != nil {
		return err
	}
	var captured *installedObservation
	p.inner.mu.Lock()
	if !p.inner.closed {
		for _, candidate := range p.inner.sessions {
			if candidate.managed == nil || candidate.close != nil {
				continue
			}
			binding, err := candidate.managed.activation()
			if err == nil && binding.Equal(activation) {
				captured = candidate.managed
				break
			}
		}
	}
	p.inner.mu.Unlock()
	if captured != nil && captured.plan != nil {
		binding, err := host.ValidatePlanUnder(ctx, captured.plan)
		if err != nil {
			return err
		}
		original, err := plugins.ProjectActivation(binding)
		if err != nil {
			return err
		}
		if !original.Equal(activation) {
			return frame.ErrStaleBinding
		}
		for _, instance := range host.Observations() {
			if instance.ID == captured.instanceID && instance.State == frame.Ready {
				return nil
			}
		}
		return frame.ErrStaleBinding
	}
	permission := p.installed.permission
	permission.mu.Lock()
	grant := permission.scope
	permission.mu.Unlock()
	binding, err := permission.Read(ctx, activation.Identity.String(), grant)
	if err != nil {
		return err
	}
	return host.ValidateBindingUnder(ctx, binding)
}
