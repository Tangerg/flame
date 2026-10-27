package pluginservices

import (
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
)

func (o *CallComposition) RestorationFailure(bindings []plugins.ContributionBinding) (bool, error) {
	for _, binding := range bindings {
		if binding.Activation == nil {
			return false, plugin.ErrInvalid
		}
		if definite, err := o.calls.pool.RestorationFailure(binding.ServerID, *binding.Activation); err != nil {
			return definite, err
		}
	}
	return false, nil
}
