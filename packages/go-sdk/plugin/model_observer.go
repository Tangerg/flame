package plugin

import (
	"github.com/Tangerg/flame/packages/go-sdk/extension"
	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	"github.com/Tangerg/flame/packages/go-sdk/scopewire"

	"github.com/Tangerg/frame"
)

func ModelObserverContract() (*plugincontract.Capability, error) { return modelObserverContract() }

func ModelObserverCallMethod() (*pluginmcp.Method[scopewire.Request, struct{}], error) {
	return pluginmcp.NewMethod[scopewire.Request, struct{}](frame.MethodConfig{
		Name: "flame.model-observer/call.v1", Kind: frame.Command, Target: extension.ScopeCall,
		Description: "Acknowledge observation of one captured model exchange.",
	})
}

func ModelObserverStreamMethod() (*pluginmcp.StreamingMethod[scopewire.Request, scopewire.ResponseDelta, struct{}], error) {
	return pluginmcp.NewStreamingMethod[scopewire.Request, scopewire.ResponseDelta, struct{}](frame.MethodConfig{
		Name: "flame.model-observer/stream.v1", Kind: frame.Command, Target: extension.ScopeCall,
		Description: "Acknowledge observed model chunks in order and confirm observation ended.",
	})
}

var modelObserverContract = publishedCapability(func() (*plugincontract.Capability, error) {
	call, err := ModelObserverCallMethod()
	if err != nil {
		return nil, err
	}
	stream, err := ModelObserverStreamMethod()
	if err != nil {
		return nil, err
	}
	return plugincontract.NewCapability(plugincontract.CapabilityConfig{Reference: "flame.model-observers.v2", Composition: plugincontract.Decoration, Scope: extension.ScopeCall}, call.Descriptor(), stream.Descriptor())
})
