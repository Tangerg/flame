package plugin

import (
	"errors"
	"sync"

	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/scope/core/chat"
)

func ToolProposalMethod() (*Method[chat.ToolCall, chat.ToolCall], error) {
	return NewMethod[chat.ToolCall, chat.ToolCall](MethodConfig{Name: "flame.tool-proposal/transform.v1", Kind: capability.Query, Target: "call", Description: "Transform a captured Tool proposal's arguments before final schema admission and approval, preserving its logical identity and Tool name."})
}

func ToolProposalContract() (*capability.Contract, error) { return toolProposalContract() }

var toolProposalContract = sync.OnceValues(func() (*capability.Contract, error) {
	method, err := ToolProposalMethod()
	if err != nil {
		return nil, err
	}
	return capability.New(capability.Config{Reference: "flame.tool-proposal.v1", Composition: capability.Decoration, Scope: "call"}, method.Descriptor())
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
