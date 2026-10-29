package plugin

import (
	"sync"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/qimu/portable"
)

// ModelObserverContract requires transparent Call/Stream observation of the
// exact remaining chain. It grants no request transformation or extra model call.
func ModelObserverContract() (*portable.Capability, error) { return modelObserverContract() }

// The published contract is immutable. Checkpoint validation shares its compiled
// schemas instead of repeating admission on every durable tree commit.
var modelObserverContract = sync.OnceValues(func() (*portable.Capability, error) {
	call, err := ModelMethod()
	if err != nil {
		return nil, err
	}
	stream, err := ModelStreamMethod()
	if err != nil {
		return nil, err
	}
	return portable.NewCapability(portable.CapabilityConfig{Reference: "flame.model-observers.v1", Composition: portable.Decoration, Scope: extension.ScopeCall}, call.Descriptor(), stream.Descriptor())
})
