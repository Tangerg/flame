package mcp

import (
	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/goalworkflow"
	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"

	"github.com/Tangerg/frame"
)

func DecideMethod() (*pluginmcp.Method[goalworkflow.Request, goalworkflow.Decision], error) {
	return pluginmcp.NewMethod[goalworkflow.Request, goalworkflow.Decision](frame.MethodConfig{Name: "flame.goal-workflow/decide.v1", Kind: frame.Query, Target: extension.ScopeInstallation, Description: "Decide a lifecycle transition for the captured objective or retain its current state."})
}
func Contract() (*plugincontract.Capability, error) {
	m, err := DecideMethod()
	if err != nil {
		return nil, err
	}
	return plugincontract.NewCapability(plugincontract.CapabilityConfig{Reference: goalworkflow.Reference, Composition: plugincontract.Selection, Scope: extension.ScopeInstallation}, m.Descriptor())
}
