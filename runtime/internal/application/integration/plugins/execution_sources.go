package plugins

import (
	"context"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/frame"
)

// ValidateExecutionSourcesUnder admits an already assembled execution. Backend
// bindings retain their original plan or business owner; immutable package
// resources retain their exact release. No source is selected in this boundary.
func ValidateExecutionSourcesUnder(
	ctx context.Context,
	host *frame.Host,
	validateBinding func(context.Context, plugin.Activation) error,
	bindings []plugin.Activation,
	resources []plugin.Dependency,
) error {
	if host == nil || validateBinding == nil {
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
			return fmt.Errorf("%w: captured package resource is no longer available", frame.ErrStaleBinding)
		}
	}
	return nil
}
