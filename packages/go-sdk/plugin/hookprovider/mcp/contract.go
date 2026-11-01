package mcp

import (
	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/hook"
	"github.com/Tangerg/flame/packages/go-sdk/plugin/hookprovider"
	qimumcp "github.com/Tangerg/qimu/mcp"
	"github.com/Tangerg/qimu/portable"
)

func DiscoverMethod() (*qimumcp.Method[hookprovider.Discovery, []hook.Hook], error) {
	return qimumcp.NewMethod[hookprovider.Discovery, []hook.Hook](qimumcp.MethodConfig{Name: "flame.hooks/discover.v1", Kind: portable.Query, Target: extension.ScopeInstallation, Description: "Read ordered lifecycle rules. Exclude all project configuration when the Runtime has not admitted its trust."})
}

func EvaluateMethod() (*qimumcp.Method[hookprovider.Evaluation, hookprovider.Advice], error) {
	return qimumcp.NewMethod[hookprovider.Evaluation, hookprovider.Advice](qimumcp.MethodConfig{Name: "flame.hooks/evaluate.v1", Kind: portable.Mutation, Target: extension.ScopeInstallation, Description: "Evaluate one captured lifecycle event and return advice. The Runtime retains trust, command material bounds and final authorization."})
}

func Contract() (*portable.Capability, error) {
	discover, err := DiscoverMethod()
	if err != nil {
		return nil, err
	}
	execute, err := EvaluateMethod()
	if err != nil {
		return nil, err
	}
	return portable.NewCapability(portable.CapabilityConfig{Reference: hookprovider.Reference, Composition: portable.Selection, Scope: extension.ScopeInstallation}, discover.Descriptor(), execute.Descriptor())
}
