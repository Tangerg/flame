package mcp

import (
	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/planworkflow"
	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	"github.com/Tangerg/frame/portable"
)

func ReviewMethod() (*pluginmcp.Method[planworkflow.Document, string], error) {
	return pluginmcp.NewMethod[planworkflow.Document, string](portable.MethodConfig{Name: "flame.plan-workflow/review.v1", Kind: portable.Query, Target: extension.ScopeInstallation, Description: "Prepare the human review of one exact immutable plan."})
}

func AuthorMethod() (*pluginmcp.Method[planworkflow.Proposal, planworkflow.Decision], error) {
	return pluginmcp.NewMethod[planworkflow.Proposal, planworkflow.Decision](portable.MethodConfig{Name: "flame.plan-workflow/author.v1", Kind: portable.Query, Target: extension.ScopeInstallation, Description: "Admit a complete checklist and issue its immutable version."})
}

func Contract() (*plugincontract.Capability, error) {
	author, err := AuthorMethod()
	if err != nil {
		return nil, err
	}
	review, err := ReviewMethod()
	if err != nil {
		return nil, err
	}
	return plugincontract.NewCapability(plugincontract.CapabilityConfig{Reference: planworkflow.Reference, Composition: plugincontract.Selection, Scope: extension.ScopeInstallation}, author.Descriptor(), review.Descriptor())
}
