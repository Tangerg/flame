package pluginservices

import (
	"context"
	"errors"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/scope/core/chat"
)

func (o *CallComposition) ResolveToolProposals(ctx context.Context) ([]plugins.ContributionBinding, error) {
	contract, err := pluginsdk.ToolProposalContract()
	if err != nil {
		return nil, err
	}
	return plugins.ResolveComposition(ctx, o.calls.installations, o.calls.compositions, contract)
}

func (o *CallComposition) ValidateToolProposals(ctx context.Context, bindings []plugins.ContributionBinding) error {
	contract, err := pluginsdk.ToolProposalContract()
	if err != nil {
		return err
	}
	return o.validateCompositionBindings(ctx, contract, bindings)
}

func (o *CallComposition) PrepareToolProposal(ctx context.Context, bindings []plugins.ContributionBinding, proposal chat.ToolCall) (chat.ToolCall, error) {
	if err := proposal.Validate(); err != nil {
		return chat.ToolCall{}, err
	}
	invocation, err := pluginmcp.InvocationFromContext(ctx)
	if err != nil {
		return chat.ToolCall{}, err
	}
	if invocation.ID != proposal.ID || invocation.Target.Scope != extension.ScopeCall || invocation.Target.Reference != proposal.ID {
		return chat.ToolCall{}, errors.New("tool proposal: calling owner differs from the captured proposal")
	}
	if err := o.ValidateToolProposals(ctx, bindings); err != nil {
		return chat.ToolCall{}, err
	}
	contract, err := pluginsdk.ToolProposalContract()
	if err != nil {
		return chat.ToolCall{}, err
	}
	method, err := pluginsdk.ToolProposalMethod()
	if err != nil {
		return chat.ToolCall{}, err
	}
	current := proposal
	for _, binding := range bindings {
		result, err := callContribution(ctx, o.calls.pool, binding, contract, method, current)
		if err != nil {
			return chat.ToolCall{}, err
		}
		if err := pluginsdk.ValidateToolProposal(current, result); err != nil {
			return chat.ToolCall{}, err
		}
		current = result
	}
	return current, nil
}
