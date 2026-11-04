package mcp

import (
	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/hook"
	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	"github.com/Tangerg/flame/packages/go-sdk/plugin/hookprovider"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	"github.com/Tangerg/frame/portable"
)

func DiscoverMethod() (*pluginmcp.Method[hookprovider.Discovery, []hook.Hook], error) {
	return pluginmcp.NewMethod[hookprovider.Discovery, []hook.Hook](portable.MethodConfig{Name: "flame.hooks/discover.v1", Kind: portable.Query, Target: extension.ScopeInstallation, Description: "Read ordered lifecycle rules. Exclude all project configuration when the Runtime has not admitted its trust."})
}

func EvaluateMethod() (*pluginmcp.Method[hookprovider.Evaluation, hookprovider.Advice], error) {
	return pluginmcp.NewMethod[hookprovider.Evaluation, hookprovider.Advice](portable.MethodConfig{Name: "flame.hooks/evaluate.v1", Kind: portable.Command, Target: extension.ScopeInstallation, Description: "Evaluate one captured lifecycle event and return advice. The Runtime retains trust, command material bounds and final authorization."})
}

func Contract() (*plugincontract.Capability, error) {
	discover, err := DiscoverMethod()
	if err != nil {
		return nil, err
	}
	execute, err := EvaluateMethod()
	if err != nil {
		return nil, err
	}
	return plugincontract.NewCapability(plugincontract.CapabilityConfig{Reference: hookprovider.Reference, Composition: plugincontract.Selection, Scope: extension.ScopeInstallation}, discover.Descriptor(), execute.Descriptor())
}
