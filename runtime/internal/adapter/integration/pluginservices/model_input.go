package pluginservices

import (
	"context"
	"errors"

	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	pluginsdk "github.com/Tangerg/flame/runtime/plugin"
	"github.com/Tangerg/scope/core/chat"
)

func (o *ModelComposition) ResolveInput(ctx context.Context) ([]plugins.ContributionBinding, error) {
	contract, err := pluginsdk.ModelInputContract()
	if err != nil {
		return nil, err
	}
	return plugins.ResolveComposition(ctx, o.calls.installations, o.calls.compositions, contract)
}
func (o *ModelComposition) ValidateInput(ctx context.Context, bindings []plugins.ContributionBinding) error {
	contract, err := pluginsdk.ModelInputContract()
	if err != nil {
		return err
	}
	return o.validateCompositionBindings(ctx, contract, bindings)
}
func (o *ModelComposition) PrepareInput(ctx context.Context, bindings []plugins.ContributionBinding, selection modelref.Selection, request *chat.Request) (*chat.Request, error) {
	if err := selection.ValidateExact(); err != nil {
		return nil, err
	}
	if request == nil {
		return nil, errors.New("model input: request is required")
	}
	if err := request.Validate(); err != nil {
		return nil, err
	}
	if _, err := pluginsdk.InvocationFromContext(ctx); err != nil {
		return nil, err
	}
	if err := o.ValidateInput(ctx, bindings); err != nil {
		return nil, err
	}
	contract, err := pluginsdk.ModelInputContract()
	if err != nil {
		return nil, err
	}
	method, err := pluginsdk.ModelInputMethod()
	if err != nil {
		return nil, err
	}
	current := request.Clone()
	for _, binding := range bindings {
		calling, err := enterModelPhase(ctx, binding)
		if err != nil {
			return nil, err
		}
		session, err := o.calls.pool.ServiceSession(ctx, binding.ServerID)
		if err != nil {
			return nil, err
		}
		if _, err := o.calls.pool.SessionActor(ctx, binding.ServerID, session); err != nil {
			return nil, err
		}
		exported, err := pluginsdk.ResolveCapability(ctx, session, binding.Export, contract)
		if err != nil {
			return nil, err
		}
		operation, err := method.Bind(exported)
		if err != nil {
			return nil, err
		}
		metadata, release, err := o.calls.pool.OpenRPC(calling, binding.ServerID, session)
		if err != nil {
			return nil, err
		}
		result, err := func() (chat.Request, error) {
			defer release()
			return operation.CallWithMetadata(calling, pluginsdk.ModelInputRequest{Selection: pluginsdk.ModelSelection{Provider: selection.Provider(), Model: selection.Model(), ReasoningEffort: selection.ReasoningEffort()}, Request: *current.Clone()}, metadata)
		}()
		if err != nil {
			return nil, err
		}
		if err := pluginsdk.ValidateModelInput(current, &result); err != nil {
			return nil, err
		}
		current = result.Clone()
	}
	return current, nil
}
