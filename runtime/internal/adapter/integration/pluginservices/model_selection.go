package pluginservices

import (
	"context"
	"errors"

	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	pluginsdk "github.com/Tangerg/flame/runtime/plugin"
	"github.com/Tangerg/scope/core/chat"
	"github.com/google/uuid"
)

// SelectModel uses only the explicitly selected installed provider. An absent
// choice preserves the caller's proposal; a withdrawn choice is a refusal.
func (o *ModelComposition) SelectModel(ctx context.Context, proposed modelref.Selection, request *chat.Request) (modelref.Selection, error) {
	if err := proposed.ValidateExact(); err != nil {
		return modelref.Selection{}, err
	}
	contract, err := pluginsdk.ModelSelectionContract()
	if err != nil {
		return modelref.Selection{}, err
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
		return modelref.Selection{}, err
	}
	if !chosen {
		return proposed, nil
	}
	ctx, err = enterModelPhase(ctx, binding)
	if err != nil {
		return modelref.Selection{}, err
	}
	live, err := o.calls.resolveContribution(ctx, plugins.CallRequest{Installation: binding.InstallationID, Digest: binding.Digest, Contribution: binding.ContributionID, Contract: contract})
	if err != nil {
		return modelref.Selection{}, err
	}
	if live != binding {
		return modelref.Selection{}, plugin.ErrStale
	}
	session, err := o.calls.pool.ServiceSession(ctx, binding.ServerID)
	if err != nil {
		return modelref.Selection{}, err
	}
	if _, err := o.calls.pool.SessionActor(ctx, binding.ServerID, session); err != nil {
		return modelref.Selection{}, err
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
		return modelref.Selection{}, err
	}
	exported, err := pluginsdk.ResolveCapability(ctx, session, binding.Export, contract)
	if err != nil {
		return modelref.Selection{}, err
	}
	method, err := pluginsdk.ModelSelectionMethod()
	if err != nil {
		return modelref.Selection{}, err
	}
	bound, err := method.Bind(exported)
	if err != nil {
		return modelref.Selection{}, err
	}
	input := pluginsdk.ModelSelectionRequest{Proposed: pluginsdk.ModelSelection{Provider: proposed.Provider(), Model: proposed.Model(), ReasoningEffort: proposed.ReasoningEffort()}}
	if request != nil {
		input.Request = request.Clone()
	}
	metadata, release, err := o.calls.pool.OpenRPC(ctx, binding.ServerID, session)
	if err != nil {
		return modelref.Selection{}, err
	}
	defer release()
	result, err := bound.CallWithMetadata(ctx, input, metadata)
	if err != nil {
		return modelref.Selection{}, err
	}
	if err := result.Validate(); err != nil {
		return modelref.Selection{}, err
	}
	selected, err := modelref.NewWithReasoningEffort(result.Provider, result.Model, result.ReasoningEffort)
	if err != nil {
		return modelref.Selection{}, errors.Join(err, errors.New("model selection: selected route is invalid"))
	}
	return selected, nil
}
