package plugins

import (
	"context"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"

	framehost "github.com/Tangerg/frame/host"
)

func ValidateExecutionSourcesUnder(
	ctx context.Context,
	host framehost.Host,
	validateBinding func(context.Context, plugin.Activation) error,
	bindings []plugin.Activation,
	resources []plugin.Dependency,
) error {
	if !host.Valid() || validateBinding == nil {
		return plugin.ErrInvalid
	}
	if err := host.Admission().CheckAdmitted(ctx); err != nil {
		return err
	}
	for _, binding := range bindings {
		if err := validateBinding(ctx, binding); err != nil {
			return err
		}
	}
	for _, dependency := range plugin.CompactDependencies(resources) {
		if err := dependency.Validate(); err != nil {
			return err
		}
		record, found := host.Installation(dependency.InstallationID.Host())
		if !found || !record.Enabled || record.Release != dependency.Digest {
			return fmt.Errorf("%w: captured package resource is no longer available", framehost.ErrStaleBinding)
		}
	}
	return nil
}
