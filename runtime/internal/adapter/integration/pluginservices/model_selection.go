package pluginservices

import (
	"context"
	"errors"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	flamemodel "github.com/Tangerg/flame/packages/go-sdk/model"
	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	"github.com/Tangerg/flame/runtime/internal/application/integration/models"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"

	framecall "github.com/Tangerg/frame/call"
	"github.com/google/uuid"
)

func (o *CallComposition) SelectModel(ctx context.Context, proposal models.SelectionInput) (flamemodel.Selection, error) {
	if err := proposal.Context.Validate(); err != nil {
		return flamemodel.Selection{}, err
	}
	if err := proposal.Proposed.ValidateExact(); err != nil {
		return flamemodel.Selection{}, err
	}
	contract, err := pluginsdk.ModelSelectionContract()
	if err != nil {
		return flamemodel.Selection{}, err
	}
	binding, chosen, err := o.selectedModelRoute(ctx, contract)
	if err != nil {
		return flamemodel.Selection{}, err
	}
	if !chosen {
		return proposal.Proposed, nil
	}
	live, err := o.calls.resolveContribution(ctx, plugins.CallRequest{Installation: binding.InstallationID, Digest: binding.Digest, Contribution: binding.ContributionID, Contract: contract})
	if err != nil {
		return flamemodel.Selection{}, err
	}
	if !live.SameContribution(binding) {
		return flamemodel.Selection{}, plugin.ErrStale
	}
	ctx, err = pluginmcp.WithInvocation(ctx, selectionInvocation(ctx))
	if err != nil {
		return flamemodel.Selection{}, err
	}
	method, err := pluginsdk.ModelSelectionMethod()
	if err != nil {
		return flamemodel.Selection{}, err
	}
	input := pluginsdk.ModelSelectionRequest{
		Context: proposal.Context,
		Proposed: pluginsdk.ModelSelection{
			Provider: proposal.Proposed.Provider(), Model: proposal.Proposed.Model(), ReasoningEffort: proposal.Proposed.ReasoningEffort(),
		},
	}
	if proposal.Context.Limits != nil {
		input.Context.Limits = new(*proposal.Context.Limits)
	}
	if proposal.Request != nil {
		input.Request = proposal.Request.Clone()
	}
	result, err := callContribution(ctx, o.calls.pool, binding, contract, method, input)
	if err != nil {
		return flamemodel.Selection{}, err
	}
	if err := result.Validate(); err != nil {
		return flamemodel.Selection{}, err
	}
	selected, err := flamemodel.NewWithReasoningEffort(result.Provider, result.Model, result.ReasoningEffort)
	if err != nil {
		return flamemodel.Selection{}, errors.Join(err, errors.New("model selection: selected route is invalid"))
	}
	return selected, nil
}

func (o *CallComposition) selectedModelRoute(ctx context.Context, contract *plugincontract.Capability) (plugins.ContributionBinding, bool, error) {
	var binding plugins.ContributionBinding
	chosen := false
	err := o.admission.Under(ctx, func() error {
		_, found, err := o.calls.selections.Get(ctx, contract.Descriptor().Reference)
		if err != nil || !found {
			return err
		}
		binding, chosen, err = plugins.SelectedContribution(ctx, o.calls.framework, o.calls.installations, o.calls.selections, contract)
		if err == nil && !chosen {
			err = plugin.ErrUnavailable
		}
		return err
	})
	return binding, chosen, err
}

func selectionInvocation(ctx context.Context) framecall.Invocation {
	id := uuid.NewString()
	target := framecall.Target{Scope: extension.ScopeCall, Reference: id}
	invocation := framecall.Invocation{ID: id, Origin: extension.Automatic, Parent: operation.Identity(ctx), Target: target}
	if parent, err := operation.Capture(ctx, target); err == nil {
		invocation.Origin, invocation.Parent = parent.Origin, parent.ID
	}
	if parent, err := pluginmcp.InvocationFromContext(ctx); err == nil {
		invocation.Origin, invocation.Parent = parent.Origin, parent.ID
	}
	return invocation
}
