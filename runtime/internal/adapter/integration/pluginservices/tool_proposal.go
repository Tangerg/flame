package pluginservices

import (
	"context"
	"errors"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
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
	invocation, err := pluginsdk.InvocationFromContext(ctx)
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
		calling, err := enterContributionPhase(ctx, binding)
		if err != nil {
			return chat.ToolCall{}, err
		}
		session, err := o.calls.pool.ServiceSession(calling, binding.ServerID)
		if err != nil {
			return chat.ToolCall{}, err
		}
		if _, err := o.calls.pool.SessionActor(calling, binding.ServerID, session); err != nil {
			return chat.ToolCall{}, err
		}
		exported, err := pluginsdk.NewCapabilityBinding(session, binding.Export, contract)
		if err != nil {
			return chat.ToolCall{}, err
		}
		operation, err := method.Bind(exported)
		if err != nil {
			return chat.ToolCall{}, err
		}
		metadata, release, err := o.calls.pool.OpenRPC(calling, binding.ServerID, session)
		if err != nil {
			return chat.ToolCall{}, err
		}
		result, err := func() (chat.ToolCall, error) {
			defer release()
			return operation.CallWithMetadata(calling, current, metadata)
		}()
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
