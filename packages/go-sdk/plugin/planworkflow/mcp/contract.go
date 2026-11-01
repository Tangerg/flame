package mcp

import (
	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/planworkflow"
	qimumcp "github.com/Tangerg/qimu/mcp"
	"github.com/Tangerg/qimu/portable"
)

func ReviewMethod() (*qimumcp.Method[planworkflow.Document, string], error) {
	return qimumcp.NewMethod[planworkflow.Document, string](qimumcp.MethodConfig{Name: "flame.plan-workflow/review.v1", Kind: portable.Query, Target: extension.ScopeInstallation, Description: "Prepare the human review of one exact immutable plan."})
}

func AuthorMethod() (*qimumcp.Method[planworkflow.Proposal, planworkflow.Decision], error) {
	return qimumcp.NewMethod[planworkflow.Proposal, planworkflow.Decision](qimumcp.MethodConfig{Name: "flame.plan-workflow/author.v1", Kind: portable.Query, Target: extension.ScopeInstallation, Description: "Admit a complete checklist and issue its immutable version."})
}

func Contract() (*portable.Capability, error) {
	author, err := AuthorMethod()
	if err != nil {
		return nil, err
	}
	review, err := ReviewMethod()
	if err != nil {
		return nil, err
	}
	return portable.NewCapability(portable.CapabilityConfig{Reference: planworkflow.Reference, Composition: portable.Selection, Scope: extension.ScopeInstallation}, author.Descriptor(), review.Descriptor())
}
