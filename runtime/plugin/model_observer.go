package plugin

import "github.com/Tangerg/flame/runtime/capability"

// ModelObserverContract requires transparent Call/Stream observation of the
// exact remaining chain. It grants no request transformation or extra model call.
func ModelObserverContract() (*capability.Contract, error) {
	call, err := ModelMethod()
	if err != nil {
		return nil, err
	}
	stream, err := ModelStreamMethod()
	if err != nil {
		return nil, err
	}
	return capability.New(capability.Config{Reference: "flame.model-observers.v1", Composition: capability.Decoration, Scope: "call"}, call.Descriptor(), stream.Descriptor())
}
