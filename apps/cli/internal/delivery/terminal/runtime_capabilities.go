package terminal

import (
	"fmt"

	"github.com/Tangerg/flame/apps/cli/internal/application/extensions"
	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

type RuntimeProfile interface {
	IdempotencyLimits() protocol.IdempotencyLimits
	Discovery() protocol.DiscoverResponse
	Supports(string) bool
	AvailableFeatureNames() []string
}

func (a *app) runtimeSupports(feature string) bool {
	return a.runtimeProfile.Supports(feature)
}

func (a *app) requireRuntimeFeature(feature string) error {
	if a.runtimeSupports(feature) {
		return nil
	}
	return fmt.Errorf("runtime capability %q was not negotiated", feature)
}

func (a *app) validateMessageCapabilities(message authoring.Message) error {
	for _, attachment := range message.Attachments {
		if attachment.Kind == protocol.ContentBlockImage {
			return a.requireRuntimeFeature(protocol.FeatureMultimodal)
		}
	}
	return nil
}

func availableWithRuntimeFeature(a *app, feature string) extensions.CommandAvailability {
	if err := a.requireRuntimeFeature(feature); err != nil {
		return extensions.CommandUnavailable(err.Error())
	}
	return extensions.CommandAvailable()
}

func requiresRuntimeFeature(feature string) func(*app) extensions.CommandAvailability {
	return func(a *app) extensions.CommandAvailability { return availableWithRuntimeFeature(a, feature) }
}
