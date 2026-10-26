package plugin

import (
	"sync"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
)

// ModelObserverContract requires transparent Call/Stream observation of the
// exact remaining chain. It grants no request transformation or extra model call.
func ModelObserverContract() (*extension.Capability, error) { return modelObserverContract() }

// The published contract is immutable. Checkpoint validation shares its compiled
// schemas instead of repeating admission on every durable tree commit.
var modelObserverContract = sync.OnceValues(func() (*extension.Capability, error) {
	call, err := ModelMethod()
	if err != nil {
		return nil, err
	}
	stream, err := ModelStreamMethod()
	if err != nil {
		return nil, err
	}
	return extension.NewCapability(extension.CapabilityConfig{Reference: "flame.model-observers.v1", Composition: extension.Decoration, Scope: "call"}, call.Descriptor(), stream.Descriptor())
})
