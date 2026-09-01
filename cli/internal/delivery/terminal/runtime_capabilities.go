package terminal

import (
	"fmt"

	"github.com/Tangerg/flame/runtime/protocol"

	"github.com/Tangerg/flame/cli/internal/domain/agent"
)

// runtimeSupports is optimistic only for backends without discovery, such as
// the scripted demo runtime. A discovered runtime is authoritative: a missing,
// disabled, or declined opt-in feature is unavailable.
func (a *app) runtimeSupports(feature string) bool {
	return a.runtimeProfile == nil || a.runtimeProfile.Supports(feature)
}

func (a *app) requireRuntimeFeature(feature string) error {
	if a.runtimeSupports(feature) {
		return nil
	}
	return fmt.Errorf("runtime capability %q was not negotiated", feature)
}

func (a *app) validateMessageCapabilities(message agent.Message) error {
	for _, attachment := range message.Attachments {
		if attachment.Kind == protocol.ContentBlockImage {
			return a.requireRuntimeFeature(protocol.FeatureMultimodal)
		}
	}
	return nil
}

func availableWithRuntimeFeature(a *app, feature string) CommandAvailability {
	if err := a.requireRuntimeFeature(feature); err != nil {
		return CommandAvailability{Reason: err.Error()}
	}
	return CommandAvailability{Enabled: true}
}
