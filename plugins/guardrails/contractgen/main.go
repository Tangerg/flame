package main

import (
	"github.com/Tangerg/flame/packages/go-sdk/extension"
	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	"github.com/Tangerg/flame/plugins/guardrails/internal/guardrail"
	"github.com/Tangerg/flame/plugins/guardrails/internal/wire"

	"github.com/Tangerg/frame"
)

func main() { plugincontract.Publish(define) }

func define() (*plugincontract.Capability, error) {
	get, err := pluginmcp.NewMethod[struct{}, guardrail.Summary](frame.MethodConfig{Name: "flame.guardrails/get-policy.v1", Kind: frame.Query, Target: extension.ScopeInstallation, Description: "Inspect the private policy revision and protected phrase count without returning protected values."})
	if err != nil {
		return nil, err
	}
	replace, err := pluginmcp.NewMethod[wire.ReplaceInput, guardrail.Summary](frame.MethodConfig{Name: "flame.guardrails/replace-policy.v1", Kind: frame.Command, Target: extension.ScopeInstallation, Description: "Replace exact, case-sensitive protected phrases at the inspected revision. Matching model-visible string fields refuse provider dispatch."})
	if err != nil {
		return nil, err
	}
	return plugincontract.NewCapability(plugincontract.CapabilityConfig{Reference: "flame.guardrails.v1", Composition: plugincontract.Collection, Scope: extension.ScopeInstallation}, get.Descriptor(), replace.Descriptor())
}
