package plugin

import (
	"errors"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"sync"

	"github.com/Tangerg/flame/qimu/portable"
	"github.com/Tangerg/scope/core/chat"
)

func ToolProposalMethod() (*qimumcp.Method[chat.ToolCall, chat.ToolCall], error) {
	return qimumcp.NewMethod[chat.ToolCall, chat.ToolCall](qimumcp.MethodConfig{Name: "flame.tool-proposal/transform.v1", Kind: portable.Query, Target: "call", Description: "Transform a captured Tool proposal's arguments before final schema admission and approval, preserving its logical identity and Tool name."})
}

func ToolProposalContract() (*portable.Capability, error) { return toolProposalContract() }

var toolProposalContract = sync.OnceValues(func() (*portable.Capability, error) {
	method, err := ToolProposalMethod()
	if err != nil {
		return nil, err
	}
	return portable.NewCapability(portable.CapabilityConfig{Reference: "flame.tool-proposal.v1", Composition: portable.Decoration, Scope: "call"}, method.Descriptor())
})

// Identity and Tool authority belong to execution. Only the final Tool owner can
// admit argument structure; the contributor cannot replace that contract.
func ValidateToolProposal(original, transformed chat.ToolCall) error {
	if err := original.Validate(); err != nil {
		return err
	}
	if err := transformed.Validate(); err != nil {
		return err
	}
	if original.ID != transformed.ID || original.Name != transformed.Name {
		return errors.New("tool proposal: transformation cannot replace call identity or Tool name")
	}
	return nil
}
