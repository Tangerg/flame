package mcp

import (
	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/goalworkflow"
	qimumcp "github.com/Tangerg/qimu/mcp"
	"github.com/Tangerg/qimu/portable"
)

func DecideMethod() (*qimumcp.Method[goalworkflow.Request, goalworkflow.Decision], error) {
	return qimumcp.NewMethod[goalworkflow.Request, goalworkflow.Decision](qimumcp.MethodConfig{Name: "flame.goal-workflow/decide.v1", Kind: portable.Query, Target: extension.ScopeInstallation, Description: "Decide a lifecycle transition for the captured objective or retain its current state."})
}
func Contract() (*portable.Capability, error) {
	m, err := DecideMethod()
	if err != nil {
		return nil, err
	}
	return portable.NewCapability(portable.CapabilityConfig{Reference: goalworkflow.Reference, Composition: portable.Selection, Scope: extension.ScopeInstallation}, m.Descriptor())
}
