package plugin

import (
	"sync"

	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
)

// ModelObserverContract requires transparent Call/Stream observation of the
// exact remaining chain. It grants no request transformation or extra model call.
func ModelObserverContract() (*plugincontract.Capability, error) { return modelObserverContract() }

// The published contract is immutable. Checkpoint validation shares its compiled
// schemas instead of repeating admission on every durable tree commit.
var modelObserverContract = sync.OnceValues(func() (*plugincontract.Capability, error) {
	call, err := ModelMethod()
	if err != nil {
		return nil, err
	}
	stream, err := ModelStreamMethod()
	if err != nil {
		return nil, err
	}
	return plugincontract.NewCapability(plugincontract.CapabilityConfig{Reference: "flame.model-observers.v1", Composition: plugincontract.Decoration, Scope: extension.ScopeCall}, call.Descriptor(), stream.Descriptor())
})
