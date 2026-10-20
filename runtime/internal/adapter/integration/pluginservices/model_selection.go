package pluginservices

import (
	"context"
	"errors"

	pluginsdk "github.com/Tangerg/flame/packages/plugin-sdk/go"
	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/internal/application/integration/models"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	modelchoice "github.com/Tangerg/flame/runtime/model"
	"github.com/google/uuid"
)

// SelectModel uses only the explicitly selected installed provider. An absent
// choice preserves the caller's proposal; a withdrawn choice is a refusal.
func (o *CallComposition) SelectModel(ctx context.Context, proposal models.SelectionInput) (modelchoice.Selection, error) {
	if err := proposal.Context.Validate(); err != nil {
		return modelchoice.Selection{}, err
	}
	if err := proposal.Proposed.ValidateExact(); err != nil {
		return modelchoice.Selection{}, err
	}
	contract, err := pluginsdk.ModelSelectionContract()
	if err != nil {
		return modelchoice.Selection{}, err
	}
	var binding plugins.ContributionBinding
	chosen := false
	err = o.admission.Under(ctx, func() error {
		_, found, err := o.calls.selections.Get(ctx, contract.Descriptor().Reference)
		if err != nil || !found {
			return err
		}
		binding, chosen, err = plugins.SelectedContribution(ctx, o.calls.installations, o.calls.selections, contract)
		if err != nil {
			return err
		}
		if !chosen {
			return plugin.ErrUnavailable
		}
		return nil
	})
	if err != nil {
		return modelchoice.Selection{}, err
	}
	if !chosen {
		return proposal.Proposed, nil
	}
	ctx, err = enterContributionPhase(ctx, binding)
	if err != nil {
		return modelchoice.Selection{}, err
	}
	live, err := o.calls.resolveContribution(ctx, plugins.CallRequest{Installation: binding.InstallationID, Digest: binding.Digest, Contribution: binding.ContributionID, Contract: contract})
	if err != nil {
		return modelchoice.Selection{}, err
	}
	if live != binding {
		return modelchoice.Selection{}, plugin.ErrStale
	}
	session, err := o.calls.pool.ServiceSession(ctx, binding.ServerID)
	if err != nil {
		return modelchoice.Selection{}, err
	}
	if _, err := o.calls.pool.SessionActor(ctx, binding.ServerID, session); err != nil {
		return modelchoice.Selection{}, err
	}
	id := uuid.NewString()
	target := capability.Target{Scope: "call", Reference: id}
	invocation := capability.Invocation{ID: id, Origin: capability.Automatic, Parent: operation.Identity(ctx), Target: target}
	if parent, err := operation.Capture(ctx, target); err == nil {
		invocation.Origin = parent.Origin
		invocation.Parent = parent.ID
	}
	if parent, err := pluginsdk.InvocationFromContext(ctx); err == nil {
		invocation.Origin = parent.Origin
		invocation.Parent = parent.ID
	}
	ctx, err = pluginsdk.WithInvocation(ctx, invocation)
	if err != nil {
		return modelchoice.Selection{}, err
	}
	exported, err := pluginsdk.ResolveCapability(ctx, session, binding.Export, contract)
	if err != nil {
		return modelchoice.Selection{}, err
	}
	method, err := pluginsdk.ModelSelectionMethod()
	if err != nil {
		return modelchoice.Selection{}, err
	}
	bound, err := method.Bind(exported)
	if err != nil {
		return modelchoice.Selection{}, err
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
	metadata, release, err := o.calls.pool.OpenRPC(ctx, binding.ServerID, session)
	if err != nil {
		return modelchoice.Selection{}, err
	}
	defer release()
	result, err := bound.CallWithMetadata(ctx, input, metadata)
	if err != nil {
		return modelchoice.Selection{}, err
	}
	if err := result.Validate(); err != nil {
		return modelchoice.Selection{}, err
	}
	selected, err := modelchoice.NewWithReasoningEffort(result.Provider, result.Model, result.ReasoningEffort)
	if err != nil {
		return modelchoice.Selection{}, errors.Join(err, errors.New("model selection: selected route is invalid"))
	}
	return selected, nil
}
